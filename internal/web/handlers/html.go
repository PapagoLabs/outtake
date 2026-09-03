// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/rs/zerolog/log"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/binding"
	"github.com/PapagoLabs/outtake/internal/config"
	"github.com/PapagoLabs/outtake/internal/database"
	"github.com/PapagoLabs/outtake/internal/media"
	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/queue"
	"github.com/PapagoLabs/outtake/internal/web/components/browse"
	"github.com/PapagoLabs/outtake/internal/web/components/clip"
	"github.com/PapagoLabs/outtake/internal/web/components/nav"
	"github.com/PapagoLabs/outtake/internal/web/components/playback"
	"github.com/PapagoLabs/outtake/internal/web/middleware"
	"github.com/PapagoLabs/outtake/internal/web/pages"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

// HTMLHandler handles HTML page requests.
type HTMLHandler struct {
	queue    *queue.Queue
	db       *database.DB
	bind     *binding.Binding
	cfg      *config.Config
	product  string
	clientID string
}

// dashStats holds dashboard clip counters.
type dashStats struct {
	total     int
	pending   int
	completed int
	failed    int
}

// NewHTMLHandler creates a new HTML handler.
func NewHTMLHandler(
	jobQueue *queue.Queue,
	db *database.DB,
	bind *binding.Binding,
	cfg *config.Config,
	product, clientID string,
) *HTMLHandler {
	// Bundle queue, database, binding, and config.
	return &HTMLHandler{
		queue:    jobQueue,
		db:       db,
		bind:     bind,
		cfg:      cfg,
		product:  product,
		clientID: clientID,
	}
}

// ClipFile streams a completed clip for in-browser playback.
func (handler *HTMLHandler) ClipFile(ctx fiber.Ctx) error {
	job := handler.lookupClip(ctx, ctx.Params(paramID))
	if job == nil || job.Status != queue.JobStatusCompleted || job.OutputPath == "" {
		return sendStatusCode(ctx, fiber.StatusNotFound)
	}

	if !clipFileExists(job.OutputPath) {
		return sendStatusCode(ctx, fiber.StatusNotFound)
	}

	err := sendRangedFile(ctx, job.OutputPath)
	if err != nil {
		return fmt.Errorf("send clip file: %w", err)
	}

	return nil
}

// ClipRow renders a single clip card for HTMX polling.
func (handler *HTMLHandler) ClipRow(ctx fiber.Ctx) error {
	job := handler.lookupClip(ctx, ctx.Params(paramID))
	if job == nil {
		return sendStatusCode(ctx, fiber.StatusNotFound)
	}

	return renderHTML(ctx, func(writer io.Writer) error {
		return clip.ClipCard(toClipItem(job, handler.clipProfileOptions(ctx))).Render(
			ctx.Context(),
			writer,
		)
	})
}

// Clips handles the clips list page request.
func (handler *HTMLHandler) Clips(ctx fiber.Ctx) error {
	return renderHTML(ctx, func(writer io.Writer) error {
		return pages.Clips(pages.ClipsProps{
			Items: handler.clipItems(ctx),
		}).Render(ctx.Context(), writer)
	})
}

// Dashboard handles the dashboard page request.
func (handler *HTMLHandler) Dashboard(ctx fiber.Ctx) error {
	jobs := handler.listJobs(ctx)
	stats := clipStats(jobs)

	return renderHTML(ctx, func(writer io.Writer) error {
		return pages.Dashboard(pages.DashboardProps{
			TotalClips:   stats.total,
			PendingClips: stats.pending,
			Completed:    stats.completed,
			Failed:       stats.failed,
			Sessions:     handler.sessionItems(),
		}).Render(ctx.Context(), writer)
	})
}

// Login handles the login page request.
func (*HTMLHandler) Login(ctx fiber.Ctx) error {
	token := sessionString(session.FromContext(ctx), middleware.SessionKeyToken)
	if token != "" {
		return redirectTo(ctx, pathRoot)
	}

	return renderHTML(ctx, func(writer io.Writer) error {
		return pages.Login(pages.LoginProps{
			AuthURL: ctx.Query("authUrl"),
			Error:   ctx.Query(queryError),
		}).Render(ctx.Context(), writer)
	})
}

// Media handles the media library page request.
func (handler *HTMLHandler) Media(ctx fiber.Ctx) error {
	query := ctx.Query("q")
	libraryID := ctx.Query(queryLibrary)
	parentID := ctx.Query("parent")
	items, libraries := handler.mediaContent(ctx, query, libraryID, parentID)
	props := view.MediaProps{
		Items:     items,
		Libraries: chooserLibraries(libraries, query, libraryID),
		Crumbs: mediaCrumbs(
			libraries,
			libraryID,
			ctx.Query("up"),
			ctx.Query("upTitle"),
			ctx.Query(queryTitle),
		),
		Query:     query,
		LibraryID: libraryID,
	}

	if ctx.Get("HX-Request") == "true" {
		return renderHTML(ctx, func(writer io.Writer) error {
			return browse.MediaResults(props).Render(ctx.Context(), writer)
		})
	}

	return renderHTML(ctx, func(writer io.Writer) error {
		return pages.Media(props).Render(ctx.Context(), writer)
	})
}

// MediaItem renders a single media item with a player and its clips.
func (handler *HTMLHandler) MediaItem(ctx fiber.Ctx) error {
	id := ctx.Params(paramID)
	item, itemErr := handler.loadMediaItem(ctx, id)
	clips := handler.clipsForMedia(ctx, id)
	tracks := handler.mediaAudioTracks(ctx, id)

	for index := range clips {
		clips[index].AudioTracks = tracks
	}

	maxDur := handler.cfg.MaxClipDurSec
	if maxDur <= 0 {
		maxDur = defaultMaxClipDur
	}

	start, err := strconv.ParseFloat(ctx.Query("start"), floatBitSize)
	if err != nil {
		start = 0
	}

	end, err := strconv.ParseFloat(ctx.Query("end"), floatBitSize)
	if err != nil || end <= start {
		end = start + defaultSegmentSecs
	}

	props := pages.MediaItemPageProps{
		ID:            id,
		Title:         id,
		Type:          "",
		Duration:      0,
		MaxDur:        maxDur,
		Clips:         clips,
		Profiles:      handler.clipProfileOptions(ctx),
		AudioTracks:   tracks,
		Error:         mediaItemError(itemErr, ctx.Query(queryError)),
		PreviewID:     ctx.Query("preview"),
		StartTime:     start,
		EndTime:       end,
		CropBlackBars: handler.cfg.CropBlackBars,
	}
	if itemErr == nil {
		props.Title = item.Title
		props.Type = item.Type
		props.Duration = item.Duration
	}

	return renderHTML(ctx, func(writer io.Writer) error {
		return pages.MediaItemPage(props).Render(ctx.Context(), writer)
	})
}

// NavLibraries renders sidebar library links.
func (handler *HTMLHandler) NavLibraries(ctx fiber.Ctx) error {
	selected := selectedLibraryID(ctx.Get("HX-Current-URL"), ctx.Query(queryLibrary))

	return renderHTML(ctx, func(writer io.Writer) error {
		return nav.NavLibraries(handler.sidebarLibraries(ctx), selected).
			Render(ctx.Context(), writer)
	})
}

// selectedLibraryID returns the library id from the nav query or the current page URL.
func selectedLibraryID(currentURL, fromQuery string) string {
	if fromQuery != "" {
		return fromQuery
	}

	if currentURL == "" {
		return ""
	}

	parsed, err := url.Parse(currentURL)
	if err != nil {
		return ""
	}

	return parsed.Query().Get(queryLibrary)
}

// NewClip handles the create-clip form.
func (handler *HTMLHandler) NewClip(ctx fiber.Ctx) error {
	start, err := strconv.ParseFloat(ctx.Query("start"), floatBitSize)
	if err != nil {
		start = 0
	}

	duration, err := strconv.ParseFloat(ctx.Query("duration"), floatBitSize)
	if err != nil {
		duration = 0
	}

	maxDur := handler.cfg.MaxClipDurSec
	if maxDur <= 0 {
		maxDur = defaultMaxClipDur
	}

	return renderHTML(ctx, func(writer io.Writer) error {
		return pages.NewClip(pages.NewClipProps{
			MediaID:       ctx.Query("mediaId"),
			MediaTitle:    ctx.Query("title"),
			MediaType:     ctx.Query("type"),
			StartTime:     start,
			Duration:      duration,
			MaxDur:        maxDur,
			Profiles:      handler.clipProfileOptions(ctx),
			AudioTracks:   handler.mediaAudioTracks(ctx, ctx.Query("mediaId")),
			CropBlackBars: handler.cfg.CropBlackBars,
		}).Render(ctx.Context(), writer)
	})
}

// Playback renders live Plex playback for a media item.
func (handler *HTMLHandler) Playback(ctx fiber.Ctx) error {
	mediaID := ctx.Params(paramID)
	props := view.Playback{
		Playing:    false,
		ViewOffset: 0,
		Title:      "",
	}

	sessions := handler.bind.Sessions()
	for index := range sessions {
		if sessions[index].MediaItem.ID != mediaID {
			continue
		}

		props.Playing = true
		props.ViewOffset = sessions[index].ViewOffset
		props.Title = sessions[index].Title

		break
	}

	return renderHTML(ctx, func(writer io.Writer) error {
		return playback.PlaybackPanel(props).Render(ctx.Context(), writer)
	})
}

// PreviewFile serves a generated segment preview.
func (handler *HTMLHandler) PreviewFile(ctx fiber.Ctx) error {
	id := ctx.Params(paramID)
	path := filepath.Join(handler.cfg.StoragePath, "previews", id+".mp4")

	err := sendRangedFile(ctx, path)
	if err != nil {
		return fmt.Errorf("send preview: %w", err)
	}

	return nil
}

// SelectServer persists the chosen Plex server.
func (handler *HTMLHandler) SelectServer(ctx fiber.Ctx) error {
	rawURL := ctx.FormValue("customUrl")
	if rawURL == "" {
		rawURL = connectionURL(
			ctx.FormValue("scheme"),
			ctx.FormValue("address"),
			ctx.FormValue("port"),
		)
	}

	err := handler.bindSelectedURL(ctx, rawURL)
	if err != nil {
		return fmt.Errorf("select server: %w", err)
	}

	return nil
}

// Servers lists discovered Plex servers.
func (handler *HTMLHandler) Servers(ctx fiber.Ctx) error {
	return renderHTML(ctx, func(writer io.Writer) error {
		return pages.Servers(pages.ServersProps{
			Servers: toServerItems(handler.discoverServers(ctx)),
			Error:   ctx.Query(queryError),
		}).Render(ctx.Context(), writer)
	})
}

// bindSelectedURL binds a Plex server from a URL and optional form name.
func (handler *HTMLHandler) bindSelectedURL(ctx fiber.Ctx, rawURL string) error {
	token := ctx.FormValue("token")
	if token == "" {
		token = sessionString(session.FromContext(ctx), middleware.SessionKeyToken)
	}

	server, ok := plex.ServerFromURL(rawURL, token)
	if !ok {
		return redirectTo(ctx, pathWithError(pathServers, "invalid server URL"))
	}

	if name := ctx.FormValue("name"); name != "" {
		server.Name = name
	}

	server.Local = false
	handler.bind.Set(server)

	err := handler.db.SaveSelectedServer(ctx.Context(), server)
	if err != nil {
		log.Warn().Err(err).Msg("failed to persist selected server")
	}

	return redirectTo(ctx, pathRoot)
}

// clipItems converts stored jobs into page models.
func (handler *HTMLHandler) clipItems(ctx fiber.Ctx) []view.ClipItem {
	jobs := handler.listJobs(ctx)
	items := make([]view.ClipItem, 0, len(jobs))

	for _, job := range jobs {
		items = append(items, toClipItem(job, handler.clipProfileOptions(ctx)))
	}

	return items
}

// clipsForMedia returns clip cards for one media id.
func (handler *HTMLHandler) clipsForMedia(ctx fiber.Ctx, mediaID string) []view.ClipItem {
	jobs, err := handler.db.ListClipsForMedia(ctx.Context(), mediaID)
	if err != nil {
		return nil
	}

	items := make([]view.ClipItem, 0, len(jobs))
	for _, job := range jobs {
		items = append(items, toClipItem(job, handler.clipProfileOptions(ctx)))
	}

	return items
}

// discoverServers lists Plex servers for the session token.
func (handler *HTMLHandler) discoverServers(ctx fiber.Ctx) []plex.Server {
	token := sessionString(session.FromContext(ctx), middleware.SessionKeyToken)
	if token == "" {
		return nil
	}

	plexClient := newBoundClient(handler.product, handler.clientID, token)

	servers, err := plexClient.DiscoverServers(ctx.Context())
	if err != nil {
		log.Warn().Err(err).Msg("discover servers failed")

		return nil
	}

	return servers
}

// listJobs returns in-memory jobs, falling back to persisted clips.
func (handler *HTMLHandler) listJobs(ctx fiber.Ctx) []*queue.Job {
	jobs := handler.queue.GetAllJobs()
	if len(jobs) > 0 {
		return jobs
	}

	stored, err := handler.db.ListClips(ctx.Context())
	if err != nil {
		return nil
	}

	return stored
}

// loadMediaItem fetches metadata for a Plex rating key.
func (handler *HTMLHandler) loadMediaItem(ctx fiber.Ctx, mediaID string) (plex.MediaItem, error) {
	plexClient, server, ok := handler.plexPair()
	if !ok {
		return plex.MediaItem{}, errNoPlexServer
	}

	item, err := plexClient.GetMediaItem(ctx.Context(), server, mediaID)
	if err != nil {
		return plex.MediaItem{}, fmt.Errorf("load media item: %w", err)
	}

	return *item, nil
}

// lookupClip finds a job in the queue or the database.
func (handler *HTMLHandler) lookupClip(ctx fiber.Ctx, id string) *queue.Job {
	job := handler.queue.GetJob(id)
	if job != nil {
		return job
	}

	stored, err := handler.db.GetClip(ctx.Context(), id)
	if err != nil {
		return nil
	}

	return stored
}

// mediaAudioTracks probes audio streams for a media item.
func (handler *HTMLHandler) mediaAudioTracks(
	ctx fiber.Ctx,
	mediaID string,
) []view.AudioTrackOption {
	// Skip probing when the media id is missing.
	if mediaID == "" {
		return nil
	}

	path, err := resolveMediaPath(
		ctx.Context(),
		handler.cfg,
		handler.bind,
		handler.product,
		handler.clientID,
		mediaID,
	)
	if err != nil {
		return nil
	}

	ffmpeg := media.NewExecFFmpeg(handler.cfg.FFmpegPath, handler.cfg.FFprobePath)

	info, err := ffmpeg.Probe(ctx.Context(), path)
	if err != nil {
		return nil
	}

	return audioTrackOptions(info.AudioTracks)
}

// audioTrackOptions maps probed streams onto select options.
func audioTrackOptions(tracks []media.AudioTrack) []view.AudioTrackOption {
	options := make([]view.AudioTrackOption, 0, len(tracks))

	for _, track := range tracks {
		options = append(options, view.AudioTrackOption{
			Index: track.Index,
			Label: audioTrackLabel(track),
		})
	}

	return options
}

// audioTrackLabel builds a short description of an audio stream.
func audioTrackLabel(track media.AudioTrack) string {
	var parts []string

	if track.Language != "" && track.Language != "und" {
		parts = append(parts, track.Language)
	}

	if track.Codec != "" {
		parts = append(parts, track.Codec)
	}

	if layout := media.ChannelLayoutName(track.Channels); layout != "" {
		parts = append(parts, layout)
	}

	if track.Title != "" {
		parts = append(parts, track.Title)
	}

	if len(parts) == 0 {
		return "Track " + strconv.Itoa(track.Index+1)
	}

	return strings.Join(parts, " · ")
}

// chooserLibraries returns library cards only for the root media view.
func chooserLibraries(libraries []view.LibraryItem, query, libraryID string) []view.LibraryItem {
	if query != "" || libraryID != "" {
		return nil
	}

	return libraries
}

// mediaContent loads libraries or media for the media page.
func (handler *HTMLHandler) mediaContent(
	ctx fiber.Ctx,
	query, libraryID, parentID string,
) ([]view.MediaItem, []view.LibraryItem) {
	// Resolve the bound Plex client before listing media.
	plexClient, server, ok := handler.plexPair()
	if !ok {
		return nil, nil
	}

	if query != "" {
		found, err := plexClient.SearchOnServer(ctx.Context(), server, query)
		if err != nil {
			log.Warn().Err(err).Msg("media search failed")

			return nil, nil
		}

		return toMediaItems(found, libraryID, "", ""), nil
	}

	libs, err := plexClient.GetLibraries(ctx.Context(), server)
	if err != nil {
		log.Warn().Err(err).Msg("list libraries failed")

		return nil, nil
	}

	if libraryID == "" {
		return nil, toLibraryItems(libs)
	}

	items, listErr := listMedia(ctx, plexClient, server, libraryID, parentID)
	if listErr != nil {
		log.Warn().Err(listErr).Msg("list media failed")

		return nil, toLibraryItems(libs)
	}

	return toMediaItems(items, libraryID, parentID, ctx.Query(queryTitle)), toLibraryItems(libs)
}

// plexPair returns a client for the currently selected server.
func (handler *HTMLHandler) plexPair() (*plex.Client, plex.Server, bool) {
	server, ok := handler.bind.Get()
	if !ok {
		return nil, plex.EmptyServer(), false
	}

	return newBoundClient(handler.product, handler.clientID, server.Token), server, true
}

// sessionItems converts live Plex sessions into page models.
func (handler *HTMLHandler) sessionItems() []pages.SessionItem {
	sessions := handler.bind.Sessions()
	items := make([]pages.SessionItem, 0, len(sessions))

	for index := range sessions {
		sess := &sessions[index]

		items = append(items, pages.SessionItem{
			ID:         sess.ID,
			MediaID:    sess.MediaItem.ID,
			Title:      sess.Title,
			ViewOffset: sess.ViewOffset,
			Duration:   sess.Duration,
		})
	}

	return items
}

// sidebarLibraries lists libraries for the sidebar.
func (handler *HTMLHandler) sidebarLibraries(ctx fiber.Ctx) []view.LibraryItem {
	plexClient, server, ok := handler.plexPair()
	if !ok {
		return nil
	}

	libs, err := plexClient.GetLibraries(ctx.Context(), server)
	if err != nil {
		return nil
	}

	return toLibraryItems(libs)
}

// clipStats summarizes jobs for the dashboard.
func clipStats(jobs []*queue.Job) dashStats {
	stats := dashStats{
		total:     len(jobs),
		pending:   0,
		completed: 0,
		failed:    0,
	}

	for _, job := range jobs {
		switch job.Status {
		case queue.JobStatusPending, queue.JobStatusProcessing:
			stats.pending++
		case queue.JobStatusCompleted:
			stats.completed++
		case queue.JobStatusFailed:
			stats.failed++
		default:
		}
	}

	return stats
}

// newBoundClient constructs a Plex client for the given token.
// ConnectionURL builds a Plex base URL from form fields.
func connectionURL(scheme, address, port string) string {
	if address == "" {
		return ""
	}

	if scheme == "" {
		scheme = "http"
	}

	if port == "" || port == "0" {
		return scheme + "://" + address
	}

	return scheme + "://" + address + ":" + port
}

// newBoundClient constructs a Plex client for the given token.
func newBoundClient(product, clientID, token string) *plex.Client {
	return plex.NewClient(plex.ClientConfig{
		Product:  product,
		ClientID: clientID,
		Token:    token,
		Timeout:  0,
		BaseURL:  "",
	})
}

// clipFileExists reports whether a clip output is present on disk.
func clipFileExists(path string) bool {
	if path == "" {
		return false
	}

	_, err := os.Stat(path)

	return err == nil
}

// clipProfileName returns a stored profile's display name.
func clipProfileName(quality string, profiles []view.ClipProfileOption) string {
	for _, profile := range profiles {
		if profile.ID == quality {
			return profile.Name
		}
	}

	return quality
}

// toClipItem maps a job onto a clips-page card.
func toClipItem(job *queue.Job, profiles []view.ClipProfileOption) view.ClipItem {
	return view.ClipItem{
		ID:            job.ID,
		Name:          job.Name,
		MediaID:       job.MediaID,
		MediaTitle:    job.MediaTitle,
		ClipType:      string(job.Type),
		Status:        string(job.Status),
		Progress:      job.Progress,
		CreatedAt:     job.CreatedAt.Format(time.RFC3339),
		StartTime:     job.StartTime,
		Duration:      job.Duration,
		Quality:       job.Quality,
		ProfileName:   clipProfileName(job.Quality, profiles),
		Profiles:      profiles,
		FileExists:    clipFileExists(job.OutputPath),
		AudioIndex:    job.AudioIndex,
		AudioTracks:   nil,
		CropBlackBars: job.CropBlackBars,
	}
}

// toLibraryItems maps Plex libraries onto page models.
func toLibraryItems(libs []plex.Library) []view.LibraryItem {
	out := make([]view.LibraryItem, 0, len(libs))
	for _, lib := range libs {
		out = append(out, view.LibraryItem{
			ID:    lib.ID,
			Title: lib.Title,
			Type:  lib.Type,
		})
	}

	return out
}

// listMedia loads a library section or the children of a show/season.
func listMedia(
	ctx fiber.Ctx,
	plexClient *plex.Client,
	server plex.Server,
	libraryID, parentID string,
) ([]plex.MediaItem, error) {
	// Prefer children of a container when parentID is set.
	if parentID != "" {
		items, err := plexClient.GetChildren(ctx.Context(), server, parentID)
		if err != nil {
			return nil, fmt.Errorf("list children: %w", err)
		}

		return items, nil
	}

	items, err := plexClient.GetMedia(ctx.Context(), server, libraryID)
	if err != nil {
		return nil, fmt.Errorf("list section: %w", err)
	}

	return items, nil
}

// mediaCrumbs builds the library / show / season trail.
func mediaCrumbs(libs []view.LibraryItem, libraryID, upID, upTitle, title string) []view.Crumb {
	crumbs := []view.Crumb{{Title: "Libraries", URL: "/media"}}
	if libraryID == "" {
		return crumbs
	}

	libTitle := libraryID
	for _, lib := range libs {
		if lib.ID == libraryID {
			libTitle = lib.Title

			break
		}
	}

	libURL := "/media?library=" + url.QueryEscape(libraryID)

	crumbs = append(crumbs, view.Crumb{Title: libTitle, URL: libURL})

	if upID != "" {
		upURL := libURL + "&parent=" + url.QueryEscape(
			upID,
		) + "&" + queryTitle + "=" + url.QueryEscape(
			upTitle,
		)

		crumbs = append(crumbs, view.Crumb{Title: upTitle, URL: upURL})
	}

	if title != "" {
		crumbs = append(crumbs, view.Crumb{Title: title, URL: ""})
	}

	return crumbs
}

// browseURL builds a drill-down link for a container item.
func browseURL(libraryID, itemID, itemTitle, parentID, parentTitle string) string {
	values := url.Values{}
	values.Set(queryLibrary, libraryID)
	values.Set("parent", itemID)
	values.Set(queryTitle, itemTitle)

	if parentID != "" {
		values.Set("up", parentID)
		values.Set("upTitle", parentTitle)
	}

	return "/media?" + values.Encode()
}

// thumbSrc rewrites a Plex thumb path onto the local cache proxy.
func thumbSrc(path string) string {
	if path == "" || !plex.ValidThumbPath(path) {
		return ""
	}

	return "/thumbs?path=" + url.QueryEscape(path)
}

// toMediaItems maps Plex media onto page models.
func toMediaItems(
	items []plex.MediaItem,
	libraryID, parentID, parentTitle string,
) []view.MediaItem {
	// Preserve input order while mapping onto page models.
	out := make([]view.MediaItem, 0, len(items))
	for _, item := range items {
		out = append(out, view.MediaItem{
			ID:        item.ID,
			Title:     item.Title,
			Type:      item.Type,
			Duration:  item.Duration,
			ThumbPath: thumbSrc(item.ThumbPath),
			Browsable: plex.IsContainerType(item.Type),
			BrowseURL: browseURL(libraryID, item.ID, item.Title, parentID, parentTitle),
		})
	}

	return out
}

// toServerItems maps discovered servers onto page models.
func toServerItems(servers []plex.Server) []pages.ServerItem {
	out := make([]pages.ServerItem, 0, len(servers))
	for _, server := range servers {
		out = append(out, pages.ServerItem{
			Name:    server.Name,
			Address: server.Address,
			Port:    server.Port,
			Scheme:  server.Scheme,
			Token:   server.Token,
			Local:   server.Local,
		})
	}

	return out
}

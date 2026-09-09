// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package html

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	fiber "github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/rs/zerolog/log"

	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/config"
	"github.com/PapagoLabs/outtake/internal/database"
	"github.com/PapagoLabs/outtake/internal/media"
	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/plex/binding"
	plexserver "github.com/PapagoLabs/outtake/internal/plex/server"
	plextitle "github.com/PapagoLabs/outtake/internal/plex/title"
	"github.com/PapagoLabs/outtake/internal/web/components/browse"
	"github.com/PapagoLabs/outtake/internal/web/components/clip"
	"github.com/PapagoLabs/outtake/internal/web/components/nav"
	"github.com/PapagoLabs/outtake/internal/web/components/playback"
	clipapi "github.com/PapagoLabs/outtake/internal/web/handlers/api/clip"
	sharedplex "github.com/PapagoLabs/outtake/internal/web/handlers/shared/plex"
	"github.com/PapagoLabs/outtake/internal/web/handlers/shared/respond"
	"github.com/PapagoLabs/outtake/internal/web/middleware"
	"github.com/PapagoLabs/outtake/internal/web/pages/auth"
	"github.com/PapagoLabs/outtake/internal/web/pages/clips"
	"github.com/PapagoLabs/outtake/internal/web/pages/dashboard"
	mediapage "github.com/PapagoLabs/outtake/internal/web/pages/media"
	"github.com/PapagoLabs/outtake/internal/web/pages/settings"
	viewclip "github.com/PapagoLabs/outtake/internal/web/view/clip"
	viewmedia "github.com/PapagoLabs/outtake/internal/web/view/media"
	viewplayback "github.com/PapagoLabs/outtake/internal/web/view/playback"
)

// HTMLHandler handles HTML page requests.
type HTMLHandler struct {
	queue    *queue.Queue
	db       *database.DB
	bind     *binding.Binding
	cfg      *config.Config
	product  string
	clientID string
	addedAt  addedAtIndexCache
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
		return respond.SendStatusCode(ctx, fiber.StatusNotFound)
	}

	if !clipFileExists(job.OutputPath) {
		return respond.SendStatusCode(ctx, fiber.StatusNotFound)
	}

	err := respond.SendRangedFile(ctx, job.OutputPath)
	if err != nil {
		return fmt.Errorf("send clip file: %w", err)
	}

	return nil
}

// ClipRow renders a single clip card for HTMX polling.
func (handler *HTMLHandler) ClipRow(ctx fiber.Ctx) error {
	job := handler.lookupClip(ctx, ctx.Params(paramID))
	if job == nil {
		return respond.SendStatusCode(ctx, fiber.StatusNotFound)
	}

	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		item := toClipItem(job, handler.clipProfileOptions(ctx), handler.clipMaxDur())

		return clip.ClipCard(item).Render(ctx.Context(), writer)
	})
}

// Clips handles the clips list page request.
func (handler *HTMLHandler) Clips(ctx fiber.Ctx) error {
	query := clipapi.ParseListQuery(ctx)
	props := clips.ClipsProps{
		Items:  handler.jobsToClipItems(ctx, clipapi.ApplyListQuery(handler.listJobs(ctx), query)),
		Status: query.Status,
		Type:   query.Type,
		Query:  query.Query,
		Sort:   query.Sort,
	}

	if wantsClipList(ctx) {
		return respond.RenderHTML(ctx, func(writer io.Writer) error {
			return clips.ClipsList(props).Render(ctx.Context(), writer)
		})
	}

	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return clips.Clips(props).Render(ctx.Context(), writer)
	})
}

// Dashboard handles the dashboard page request.
func (handler *HTMLHandler) Dashboard(ctx fiber.Ctx) error {
	jobs := handler.listJobs(ctx)
	stats := clipStats(jobs)

	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return dashboard.Dashboard(dashboard.DashboardProps{
			TotalClips:   stats.total,
			PendingClips: stats.pending,
			Completed:    stats.completed,
			Failed:       stats.failed,
			Sessions:     handler.sessionItems(),
		}).Render(ctx.Context(), writer)
	})
}

// DashboardSessions renders the live-sessions fragment for HTMX polling.
func (handler *HTMLHandler) DashboardSessions(ctx fiber.Ctx) error {
	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return dashboard.LiveSessions(handler.sessionItems()).Render(ctx.Context(), writer)
	})
}

// Login handles the login page request.
func (*HTMLHandler) Login(ctx fiber.Ctx) error {
	token := respond.SessionString(session.FromContext(ctx), middleware.SessionKeyToken)
	if token != "" {
		return respond.RedirectTo(ctx, respond.PathRoot)
	}

	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return auth.Login(auth.LoginProps{
			AuthURL: ctx.Query("authUrl"),
			Error:   ctx.Query(respond.QueryError),
		}).Render(ctx.Context(), writer)
	})
}

// Media handles the media library page request.
//
// Parameters:
//   - ctx: Request with library browse query and optional HX-Target.
//
// Returns:
//   - err: Non-nil when rendering fails.
func (handler *HTMLHandler) Media(ctx fiber.Ctx) error {
	query := parseMediaListQuery(ctx)
	props := mediaPageProps(handler, ctx, query)

	err := renderMediaPage(ctx, &props)
	if err != nil {
		return fmt.Errorf("render media: %w", err)
	}

	return nil
}

// mediaPageProps builds library browse props for the current request.
//
// Parameters:
//   - handler: HTML handler with Plex access.
//   - ctx: Request with library browse query.
//   - query: Normalized browse state.
//
// Returns:
//   - props: Media library page model.
func mediaPageProps(
	handler *HTMLHandler,
	ctx fiber.Ctx,
	query mediaListQuery,
) viewmedia.MediaProps {
	var letters []plex.LetterIndex

	if !wantsMediaMore(ctx) && !wantsMediaPrev(ctx) {
		letters = handler.mediaLetters(ctx, query)
	}

	window := query.window(letters)
	_, _, hasServer := handler.plexPair()
	items, libraries, total := handler.mediaContent(ctx, query, window.Start, window.Size)

	return viewmedia.MediaProps{
		Items:     items,
		Libraries: chooserLibraries(libraries, query.Query, query.LibraryID),
		Crumbs: mediaCrumbs(
			libraries,
			query.LibraryID,
			ctx.Query(respond.QueryUp),
			ctx.Query(respond.QueryUpTitle),
			ctx.Query(respond.QueryTitle),
		),
		Letters:     thinJumpIndexes(toLetterIndexes(letters), query.Sort),
		Query:       query.Query,
		LibraryID:   query.LibraryID,
		ParentID:    query.ParentID,
		ParentTitle: ctx.Query(respond.QueryTitle),
		UpID:        ctx.Query(respond.QueryUp),
		UpTitle:     ctx.Query(respond.QueryUpTitle),
		Sort:        query.Sort,
		Letter:      query.Letter,
		Start:       window.Start,
		Total:       total,
		PageSize:    respond.MediaPageSize,
		HasServer:   hasServer,
	}
}

// renderMediaPage writes the media library page or an HTMX fragment.
//
// Parameters:
//   - ctx: Request with an optional HX-Target.
//   - props: Media library page model.
//
// Returns:
//   - err: Non-nil when rendering fails.
func renderMediaPage(ctx fiber.Ctx, props *viewmedia.MediaProps) error {
	switch {
	case wantsMediaPrev(ctx):
		return respond.RenderHTML(ctx, func(writer io.Writer) error {
			return browse.MediaPrev(*props).Render(ctx.Context(), writer)
		})
	case wantsMediaMore(ctx):
		return respond.RenderHTML(ctx, func(writer io.Writer) error {
			return browse.MediaMore(*props).Render(ctx.Context(), writer)
		})
	case wantsMediaResults(ctx):
		return respond.RenderHTML(ctx, func(writer io.Writer) error {
			return browse.MediaBrowse(*props).Render(ctx.Context(), writer)
		})
	default:
		return respond.RenderHTML(ctx, func(writer io.Writer) error {
			return mediapage.Media(*props).Render(ctx.Context(), writer)
		})
	}
}

// MediaItem renders a single media item with a player and its clips.
func (handler *HTMLHandler) MediaItem(ctx fiber.Ctx) error {
	id := ctx.Params(paramID)
	item, itemErr := handler.loadMediaItem(ctx, id)
	query := clipapi.ParseListQuery(ctx)
	tracks := handler.mediaAudioTracks(ctx, id)
	clips := handler.clipsForMedia(ctx, id)

	for index := range clips {
		clips[index].AudioTracks = tracks
	}

	maxDur := handler.cfg.MaxClipDurSec
	if maxDur <= 0 {
		maxDur = respond.DefaultMaxClipDur
	}

	start, err := strconv.ParseFloat(ctx.Query("start"), respond.FloatBitSize)
	if err != nil {
		start = 0
	}

	end, err := strconv.ParseFloat(ctx.Query("end"), respond.FloatBitSize)
	if err != nil || end <= start {
		end = start + respond.DefaultSegmentSecs
	}

	props := mediapage.MediaItemPageProps{
		ID:            id,
		Title:         id,
		Type:          "",
		Duration:      0,
		MaxDur:        maxDur,
		Clips:         clips,
		ClipStatus:    query.Status,
		ClipType:      query.Type,
		ClipQuery:     query.Query,
		ClipSort:      query.Sort,
		Profiles:      handler.clipProfileOptions(ctx),
		AudioTracks:   tracks,
		Error:         respond.MediaItemError(itemErr, ctx.Query(respond.QueryError)),
		PreviewID:     ctx.Query("preview"),
		StartTime:     start,
		EndTime:       end,
		CropBlackBars: handler.cfg.CropBlackBars,
		WebSafeColor:  previewWebSafeColor(ctx, handler.cfg.WebSafeColor),
		Crumbs:        nil,
		LibraryID:     "",
	}
	if itemErr == nil {
		props.Title = plextitle.Display(item)
		props.Type = item.Type
		props.Duration = item.Duration
		props.LibraryID = item.LibraryID
		props.Crumbs = itemCrumbs(item, handler.sidebarLibraries(ctx))
	}

	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return mediapage.MediaItemPage(props).Render(ctx.Context(), writer)
	})
}

// previewWebSafeColor prefers the preview redirect query over the config default.
//
// Parameters:
//   - ctx: Incoming page request.
//   - fallback: Config default when the query is omitted.
//
// Returns:
//   - checked: True when the New export web-safe color checkbox should be on.
func previewWebSafeColor(ctx fiber.Ctx, fallback bool) bool {
	raw := ctx.Query(respond.QueryWebSafeColor)
	if raw == "" {
		return fallback
	}

	return raw == formChecked
}

// MediaItemClips renders the media-item clip list fragment for HTMX swaps.
func (handler *HTMLHandler) MediaItemClips(ctx fiber.Ctx) error {
	id := ctx.Params(paramID)
	query := clipapi.ParseListQuery(ctx)
	tracks := handler.mediaAudioTracks(ctx, id)
	clips := handler.clipsForMedia(ctx, id)

	for index := range clips {
		clips[index].AudioTracks = tracks
	}

	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return mediapage.ItemClipList(clips, query.Filtered()).Render(ctx.Context(), writer)
	})
}

// NavLibraries renders sidebar library links.
func (handler *HTMLHandler) NavLibraries(ctx fiber.Ctx) error {
	selected := selectedLibraryID(ctx.Get("HX-Current-URL"), ctx.Query(respond.QueryLibrary))

	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return nav.NavLibraries(handler.sidebarLibraries(ctx), selected).
			Render(ctx.Context(), writer)
	})
}

// wantsMediaResults reports whether the request should swap the media browse pane.
//
// Parameters:
//   - ctx: Request context with an optional HX-Target header.
//
// Returns:
//   - ok: True when HTMX is targeting #media-browse.
func wantsMediaResults(ctx fiber.Ctx) bool {
	return respond.HxTargetID(ctx.Get(respond.HeaderHXTarget)) == "media-browse"
}

// wantsMediaMore reports whether the request should append the next poster page.
//
// Parameters:
//   - ctx: Request context with an optional HX-Target header.
//
// Returns:
//   - ok: True when HTMX is targeting #media-more.
func wantsMediaMore(ctx fiber.Ctx) bool {
	return respond.HxTargetID(ctx.Get(respond.HeaderHXTarget)) == "media-more"
}

// wantsMediaPrev reports whether the request should prepend the previous poster page.
//
// Parameters:
//   - ctx: Request context with an optional HX-Target header.
//
// Returns:
//   - ok: True when HTMX is targeting #media-prev.
func wantsMediaPrev(ctx fiber.Ctx) bool {
	return respond.HxTargetID(ctx.Get(respond.HeaderHXTarget)) == "media-prev"
}

// wantsClipList reports whether the request should swap the clips list only.
//
// Parameters:
//   - ctx: Request context with an optional HX-Target header.
//
// Returns:
//   - True when HTMX is targeting #clip-list.
func wantsClipList(ctx fiber.Ctx) bool {
	return respond.HxTargetID(ctx.Get(respond.HeaderHXTarget)) == "clip-list"
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

	return parsed.Query().Get(respond.QueryLibrary)
}

// NewClip sends clip-now links to the media item editor.
func (*HTMLHandler) NewClip(ctx fiber.Ctx) error {
	mediaID := ctx.Query("mediaId")
	if mediaID == "" {
		return respond.RedirectTo(ctx, respond.PathMedia)
	}

	values := url.Values{}
	if start := ctx.Query(respond.QueryStart); start != "" {
		values.Set(respond.QueryStart, start)
	}

	return respond.RedirectTo(ctx, mediaItemLocation(mediaID, values))
}

// mediaItemLocation builds /media/item/:id with a path-escaped id.
func mediaItemLocation(mediaID string, values url.Values) string {
	location := "/media/item/" + url.PathEscape(mediaID)
	if encoded := values.Encode(); encoded != "" {
		location += "?" + encoded
	}

	return location
}

// Playback renders live Plex playback for a media item.
func (handler *HTMLHandler) Playback(ctx fiber.Ctx) error {
	mediaID := ctx.Params(paramID)
	props := viewplayback.Playback{
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

	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return playback.PlaybackPanel(props).Render(ctx.Context(), writer)
	})
}

// PreviewFile serves a generated segment preview.
func (handler *HTMLHandler) PreviewFile(ctx fiber.Ctx) error {
	id := ctx.Params(paramID)
	path := filepath.Join(handler.cfg.StoragePath, "previews", id+".mp4")

	err := respond.SendRangedFile(ctx, path)
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
	current, _ := handler.bind.Get()

	return respond.RenderHTML(ctx, func(writer io.Writer) error {
		return settings.Servers(settings.ServersProps{
			Servers: toServerItems(handler.discoverServers(ctx), current),
			Error:   ctx.Query(respond.QueryError),
		}).Render(ctx.Context(), writer)
	})
}

// bindSelectedURL binds a Plex server from a URL and optional form name.
func (handler *HTMLHandler) bindSelectedURL(ctx fiber.Ctx, rawURL string) error {
	token := ctx.FormValue("token")
	if token == "" {
		token = respond.SessionString(session.FromContext(ctx), middleware.SessionKeyToken)
	}

	server, ok := plexserver.ServerFromURL(rawURL, token)
	if !ok {
		return respond.RedirectTo(ctx, respond.PathWithError(respond.PathServers, "invalid server URL"))
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

	return respond.RedirectTo(ctx, respond.PathRoot)
}

// clipMaxDur is the configured clip duration cap, or the default when unset.
func (handler *HTMLHandler) clipMaxDur() int {
	if handler.cfg != nil && handler.cfg.MaxClipDurSec > 0 {
		return handler.cfg.MaxClipDurSec
	}

	return respond.DefaultMaxClipDur
}

// clipsForMedia returns clip cards for one media id.
func (handler *HTMLHandler) clipsForMedia(ctx fiber.Ctx, mediaID string) []viewclip.ClipItem {
	jobs, err := handler.db.ListClipsForMedia(ctx.Context(), mediaID)
	if err != nil {
		return nil
	}

	return handler.jobsToClipItems(ctx, clipapi.ApplyListQuery(jobs, clipapi.ParseListQuery(ctx)))
}

// discoverServers lists Plex servers for the session token.
func (handler *HTMLHandler) discoverServers(ctx fiber.Ctx) []plex.Server {
	token := respond.SessionString(session.FromContext(ctx), middleware.SessionKeyToken)
	if token == "" {
		return nil
	}

	plexClient := sharedplex.NewBoundClient(handler.product, handler.clientID, token)

	servers, err := plexClient.DiscoverServers(ctx.Context())
	if err != nil {
		log.Warn().Err(err).Msg("discover servers failed")

		return nil
	}

	return servers
}

// jobsToClipItems converts jobs into page models.
func (handler *HTMLHandler) jobsToClipItems(ctx fiber.Ctx, jobs []*queue.Job) []viewclip.ClipItem {
	items := make([]viewclip.ClipItem, 0, len(jobs))

	for _, job := range jobs {
		items = append(
			items,
			toClipItem(job, handler.clipProfileOptions(ctx), handler.clipMaxDur()),
		)
	}

	return items
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
) []viewclip.AudioTrackOption {
	// Skip probing when the media id is missing.
	if mediaID == "" {
		return nil
	}

	path, err := clipapi.ResolveMediaPath(
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
func audioTrackOptions(tracks []media.AudioTrack) []viewclip.AudioTrackOption {
	options := make([]viewclip.AudioTrackOption, 0, len(tracks))

	for _, track := range tracks {
		options = append(options, viewclip.AudioTrackOption{
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
func chooserLibraries(libraries []viewmedia.LibraryItem, query, libraryID string) []viewmedia.LibraryItem {
	if query != "" || libraryID != "" {
		return nil
	}

	return libraries
}

// mediaContent loads libraries or media for the media page.
//
// Parameters:
//   - ctx: Request context.
//   - query: Normalized browse state.
//   - start: Container offset.
//   - size: Page size.
//
// Returns:
//   - items: Media cards for the window.
//   - libraries: Library chooser entries.
//   - total: Total matching items.
func (handler *HTMLHandler) mediaContent(
	ctx fiber.Ctx,
	query mediaListQuery,
	start, size int,
) ([]viewmedia.MediaItem, []viewmedia.LibraryItem, int) {
	plexClient, server, ok := handler.plexPair()
	if !ok {
		return nil, nil, 0
	}

	if query.Query != "" {
		return searchMediaContent(ctx, plexClient, server, query.Query, query.LibraryID)
	}

	return listMediaContent(ctx, plexClient, server, query, start, size)
}

// mediaLetters loads jump-rail buckets for a library root.
//
// Parameters:
//   - ctx: Request context.
//   - query: Normalized browse state.
//
// Returns:
//   - letters: Title, year, or added-at buckets for the jump rail.
func (handler *HTMLHandler) mediaLetters(
	ctx fiber.Ctx,
	query mediaListQuery,
) []plex.LetterIndex {
	if !query.showJumpIndex() {
		return nil
	}

	plexClient, server, ok := handler.plexPair()
	if !ok {
		return nil
	}

	switch query.Sort {
	case mediaSortAddedDesc, mediaSortAddedAsc:
		return loadAddedAtIndexes(
			ctx.Context(),
			&handler.addedAt,
			plexClient,
			server,
			query.LibraryID,
			query.Sort,
		)
	case mediaSortYearDesc, mediaSortYearAsc:
		index, err := plexClient.GetYears(ctx.Context(), server, query.LibraryID)
		if err != nil {
			log.Warn().Err(err).Msg("list years failed")

			return nil
		}

		return orderJumpIndex(index, query.Sort)
	default:
		index, err := plexClient.GetFirstCharacters(ctx.Context(), server, query.LibraryID)
		if err != nil {
			log.Warn().Err(err).Msg("list first characters failed")

			return nil
		}

		return orderJumpIndex(index, query.Sort)
	}
}

// searchMediaContent runs a Plex hub search and keeps library names for crumbs.
func searchMediaContent(
	ctx fiber.Ctx,
	plexClient *plex.Client,
	server plex.Server,
	query, libraryID string,
) ([]viewmedia.MediaItem, []viewmedia.LibraryItem, int) {
	found, err := plexClient.SearchOnServer(ctx.Context(), server, query, libraryID)
	if err != nil {
		log.Warn().Err(err).Msg("media search failed")

		return nil, nil, 0
	}

	libs, libErr := plexClient.GetLibraries(ctx.Context(), server)
	if libErr != nil {
		log.Warn().Err(libErr).Msg("list libraries failed")

		return toMediaItems(found, libraryID, "", ""), nil, len(found)
	}

	return toMediaItems(found, libraryID, "", ""), toLibraryItems(libs), len(found)
}

// listMediaContent lists a library, a container, or the library chooser.
//
// Parameters:
//   - ctx: Request context.
//   - plexClient: PMS client.
//   - server: PMS to query.
//   - query: Normalized browse state.
//   - start: Container offset.
//   - size: Page size.
//
// Returns:
//   - items: Media cards for the window.
//   - libraries: Library chooser entries.
//   - total: Total matching items.
func listMediaContent(
	ctx fiber.Ctx,
	plexClient *plex.Client,
	server plex.Server,
	query mediaListQuery,
	start, size int,
) ([]viewmedia.MediaItem, []viewmedia.LibraryItem, int) {
	libs, err := plexClient.GetLibraries(ctx.Context(), server)
	if err != nil {
		log.Warn().Err(err).Msg("list libraries failed")

		return nil, nil, 0
	}

	if query.LibraryID == "" {
		return nil, toLibraryItems(libs), 0
	}

	page, listErr := listMediaPage(ctx, plexClient, server, query, start, size)
	if listErr != nil {
		log.Warn().Err(listErr).Msg("list media failed")

		return nil, toLibraryItems(libs), 0
	}

	return toMediaItems(
			page.Items,
			query.LibraryID,
			query.ParentID,
			ctx.Query(respond.QueryTitle),
		), toLibraryItems(
			libs,
		), page.Total
}

// plexPair returns a client for the currently selected server.
func (handler *HTMLHandler) plexPair() (*plex.Client, plex.Server, bool) {
	server, ok := handler.bind.Get()
	if !ok {
		return nil, plex.EmptyServer(), false
	}

	return sharedplex.NewBoundClient(handler.product, handler.clientID, server.Token), server, true
}

// sessionItems converts live Plex sessions into page models.
func (handler *HTMLHandler) sessionItems() []dashboard.SessionItem {
	sessions := handler.bind.Sessions()
	items := make([]dashboard.SessionItem, 0, len(sessions))

	for index := range sessions {
		sess := &sessions[index]

		parts, year := sessionTitleParts(sess.MediaItem)

		items = append(items, dashboard.SessionItem{
			ID:         sess.ID,
			MediaID:    sess.MediaItem.ID,
			Parts:      parts,
			Year:       year,
			ViewOffset: sess.ViewOffset,
			Duration:   sess.Duration,
		})
	}

	return items
}

// sidebarLibraries lists libraries for the sidebar.
func (handler *HTMLHandler) sidebarLibraries(ctx fiber.Ctx) []viewmedia.LibraryItem {
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

// clipFileExists reports whether a clip output is present on disk.
func clipFileExists(path string) bool {
	if path == "" {
		return false
	}

	_, err := os.Stat(path)

	return err == nil
}

// clipProfileName returns a stored profile's display name.
func clipProfileName(quality string, profiles []viewclip.ClipProfileOption) string {
	for _, profile := range profiles {
		if profile.ID == quality {
			return profile.Name
		}
	}

	return quality
}

// toClipItem maps a job onto a clips-page card.
func toClipItem(job *queue.Job, profiles []viewclip.ClipProfileOption, maxDur int) viewclip.ClipItem {
	return viewclip.ClipItem{
		ID:            job.ID,
		Name:          job.Name,
		MediaID:       job.MediaID,
		MediaTitle:    job.MediaTitle,
		ClipType:      string(job.Type),
		Status:        string(job.Status),
		Progress:      job.Progress,
		CreatedAt:     formatClipCreated(job.CreatedAt),
		Error:         job.Error,
		StartTime:     job.StartTime,
		Duration:      job.Duration,
		Quality:       job.Quality,
		ProfileName:   clipProfileName(job.Quality, profiles),
		Profiles:      profiles,
		FileExists:    clipFileExists(job.OutputPath),
		AudioIndex:    job.AudioIndex,
		AudioTracks:   nil,
		CropBlackBars: job.CropBlackBars,
		WebSafeColor:  job.WebSafeColor,
		Width:         job.Width,
		FPS:           job.FPS,
		MaxDur:        maxDur,
	}
}

// toLibraryItems maps Plex libraries onto page models.
func toLibraryItems(libs []plex.Library) []viewmedia.LibraryItem {
	out := make([]viewmedia.LibraryItem, 0, len(libs))
	for _, lib := range libs {
		out = append(out, viewmedia.LibraryItem{
			ID:        lib.ID,
			Title:     lib.Title,
			Type:      lib.Type,
			ThumbPath: thumbSrc(lib.ThumbPath),
		})
	}

	return out
}

// listMediaPage loads one page of a library section or container children.
//
// Parameters:
//   - ctx: Request context.
//   - plexClient: PMS client.
//   - server: PMS to query.
//   - query: Normalized browse state.
//   - start: Container offset.
//   - size: Page size.
//
// Returns:
//   - page: Items and total size for the requested window.
//   - err: Non-nil when the PMS request fails.
func listMediaPage(
	ctx fiber.Ctx,
	plexClient *plex.Client,
	server plex.Server,
	query mediaListQuery,
	start, size int,
) (plex.MediaPage, error) {
	if query.ParentID != "" {
		page, err := plexClient.GetChildrenPage(ctx.Context(), server, query.ParentID, start, size)
		if err != nil {
			return plex.MediaPage{}, fmt.Errorf("list children: %w", err)
		}

		return page, nil
	}

	page, err := plexClient.GetMediaPage(
		ctx.Context(),
		server,
		query.LibraryID,
		start,
		size,
		plexMediaSort(query.Sort),
	)
	if err != nil {
		return plex.MediaPage{}, fmt.Errorf("list section: %w", err)
	}

	return page, nil
}

// mediaCrumbs builds the library / show / season trail.
func mediaCrumbs(libs []viewmedia.LibraryItem, libraryID, upID, upTitle, title string) []viewmedia.Crumb {
	crumbs := []viewmedia.Crumb{{Title: "Libraries", URL: respond.PathMedia}}
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

	libURL := respond.PathMedia + "?library=" + url.QueryEscape(libraryID)

	crumbs = append(crumbs, viewmedia.Crumb{Title: libTitle, URL: libURL})

	if upID != "" {
		upURL := libURL + "&parent=" + url.QueryEscape(
			upID,
		) + "&" + respond.QueryTitle + "=" + url.QueryEscape(
			upTitle,
		)

		crumbs = append(crumbs, viewmedia.Crumb{Title: upTitle, URL: upURL})
	}

	if title != "" {
		crumbs = append(crumbs, viewmedia.Crumb{Title: title, URL: ""})
	}

	return crumbs
}

// sessionTitleParts builds dashboard title crumbs for a live session.
func sessionTitleParts(item plex.MediaItem) ([]viewmedia.Crumb, int) {
	if item.Type != plex.TypeEpisode {
		if item.Title == "" {
			return displayTitleCrumb(item), 0
		}

		return []viewmedia.Crumb{{Title: item.Title, URL: sessionItemURL(item.ID)}}, item.Year
	}

	var parts []viewmedia.Crumb

	if item.GrandparentTitle != "" {
		parts = append(parts, viewmedia.Crumb{
			Title: item.GrandparentTitle,
			URL: sessionBrowseURL(
				item.LibraryID,
				item.GrandparentID,
				item.GrandparentTitle,
				"",
				"",
			),
		})
	}

	if label := seasonSessionLabel(item); label != "" {
		parts = append(parts, viewmedia.Crumb{
			Title: label,
			URL: sessionBrowseURL(
				item.LibraryID,
				item.ParentID,
				label,
				item.GrandparentID,
				item.GrandparentTitle,
			),
		})
	}

	if item.Title != "" {
		parts = append(parts, viewmedia.Crumb{Title: item.Title, URL: sessionItemURL(item.ID)})
	}

	if len(parts) == 0 {
		return displayTitleCrumb(item), 0
	}

	return parts, 0
}

// displayTitleCrumb is a plain-text fallback when structured parts are missing.
func displayTitleCrumb(item plex.MediaItem) []viewmedia.Crumb {
	title := plextitle.Display(item)
	if title == "" {
		return nil
	}

	return []viewmedia.Crumb{{Title: title}}
}

// seasonSessionLabel returns ParentTitle, or Season N from ParentIndex.
func seasonSessionLabel(item plex.MediaItem) string {
	if item.ParentTitle != "" {
		return item.ParentTitle
	}

	if item.ParentIndex > 0 {
		return "Season " + strconv.Itoa(item.ParentIndex)
	}

	return ""
}

// sessionBrowseURL returns a container browse URL when library and item ids exist.
func sessionBrowseURL(libraryID, itemID, itemTitle, parentID, parentTitle string) string {
	if libraryID == "" || itemID == "" {
		return ""
	}

	return browseURL(libraryID, itemID, itemTitle, parentID, parentTitle)
}

// sessionItemURL returns the media item path when id is set.
func sessionItemURL(id string) string {
	if id == "" {
		return ""
	}

	return mediaItemLocation(id, url.Values{})
}

// browseURL builds a drill-down link for a container item.
func browseURL(libraryID, itemID, itemTitle, parentID, parentTitle string) string {
	values := url.Values{}
	values.Set(respond.QueryLibrary, libraryID)
	values.Set(respond.QueryParent, itemID)
	values.Set(respond.QueryTitle, itemTitle)

	if parentID != "" {
		values.Set(respond.QueryUp, parentID)
		values.Set(respond.QueryUpTitle, parentTitle)
	}

	return respond.PathMedia + "?" + values.Encode()
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
) []viewmedia.MediaItem {
	// Preserve input order while mapping onto page models.
	out := make([]viewmedia.MediaItem, 0, len(items))
	for index := range items {
		item := items[index]
		libID := libraryID
		if libID == "" {
			libID = item.LibraryID
		}

		episodeLabel := ""
		if item.Type == plex.TypeEpisode {
			episodeLabel = plextitle.EpisodeCode(item.ParentIndex, item.Index)
		}

		out = append(out, viewmedia.MediaItem{
			ID:           item.ID,
			Title:        item.Title,
			Type:         item.Type,
			Duration:     item.Duration,
			ThumbPath:    thumbSrc(item.ThumbPath),
			Browsable:    plex.IsContainerType(item.Type),
			BrowseURL:    browseURL(libID, item.ID, item.Title, parentID, parentTitle),
			Year:         item.Year,
			Index:        item.Index,
			ParentIndex:  item.ParentIndex,
			ShowTitle:    item.GrandparentTitle,
			EpisodeLabel: episodeLabel,
			TitleSort:    item.TitleSort,
			AddedAt:      item.AddedAt,
		})
	}

	return out
}

// toServerItems maps discovered servers onto page models.
func toServerItems(servers []plex.Server, current plex.Server) []settings.ServerItem {
	out := make([]settings.ServerItem, 0, len(servers))
	for _, server := range servers {
		out = append(out, settings.ServerItem{
			Name:     server.Name,
			Address:  server.Address,
			Port:     server.Port,
			Scheme:   server.Scheme,
			Token:    server.Token,
			Local:    server.Local,
			Selected: plexserver.SameConnection(server, current),
		})
	}

	return out
}

// clipMatchesStatus reports whether a clip belongs to a status filter.
func clipMatchesStatus(itemStatus, want string) bool {
	switch want {
	case viewclip.ClipStatusPending:
		return itemStatus == viewclip.ClipStatusPending || itemStatus == viewclip.ClipStatusProcessing
	default:
		return itemStatus == want
	}
}

// formatClipCreated renders a clip timestamp for display.
func formatClipCreated(created time.Time) string {
	if created.IsZero() {
		return ""
	}

	return created.UTC().Format("Jan 2, 2006 3:04 PM")
}

// pageStart parses a non-negative pagination offset.
func pageStart(raw string) int {
	start, err := strconv.Atoi(raw)
	if err != nil || start < 0 {
		return 0
	}

	return start
}

// itemCrumbs builds Libraries / library / show / season / title for a media item.
func itemCrumbs(item plex.MediaItem, libs []viewmedia.LibraryItem) []viewmedia.Crumb {
	crumbs := []viewmedia.Crumb{{Title: "Libraries", URL: respond.PathMedia}}

	crumbs = appendLibraryCrumb(crumbs, item, libs)
	crumbs = appendShowCrumbs(crumbs, item)
	crumbs = append(crumbs, viewmedia.Crumb{Title: item.Title, URL: ""})

	return crumbs
}

// appendLibraryCrumb adds the owning library when its id is known.
func appendLibraryCrumb(
	crumbs []viewmedia.Crumb,
	item plex.MediaItem,
	libs []viewmedia.LibraryItem,
) []viewmedia.Crumb {
	if item.LibraryID == "" {
		return crumbs
	}

	title := item.LibraryTitle
	for _, lib := range libs {
		if lib.ID == item.LibraryID {
			title = lib.Title

			break
		}
	}

	if title == "" {
		title = item.LibraryID
	}

	return append(crumbs, viewmedia.Crumb{
		Title: title,
		URL:   respond.PathMedia + "?library=" + url.QueryEscape(item.LibraryID),
	})
}

// appendShowCrumbs adds show and season links for episodes.
func appendShowCrumbs(crumbs []viewmedia.Crumb, item plex.MediaItem) []viewmedia.Crumb {
	if item.GrandparentID != "" && item.GrandparentTitle != "" {
		crumbs = append(crumbs, viewmedia.Crumb{
			Title: item.GrandparentTitle,
			URL:   browseURL(item.LibraryID, item.GrandparentID, item.GrandparentTitle, "", ""),
		})
	}

	if item.Type != plex.TypeEpisode || item.ParentID == "" {
		return appendSeasonShowCrumb(crumbs, item)
	}

	seasonTitle := item.ParentTitle
	if seasonTitle == "" {
		seasonTitle = "Season"
	}

	return append(crumbs, viewmedia.Crumb{
		Title: seasonTitle,
		URL: browseURL(
			item.LibraryID,
			item.ParentID,
			seasonTitle,
			item.GrandparentID,
			item.GrandparentTitle,
		),
	})
}

// appendSeasonShowCrumb adds the parent show for a season item.
func appendSeasonShowCrumb(crumbs []viewmedia.Crumb, item plex.MediaItem) []viewmedia.Crumb {
	if item.Type != plex.TypeSeason || item.ParentID == "" || item.ParentTitle == "" {
		return crumbs
	}

	return append(crumbs, viewmedia.Crumb{
		Title: item.ParentTitle,
		URL:   browseURL(item.LibraryID, item.ParentID, item.ParentTitle, "", ""),
	})
}

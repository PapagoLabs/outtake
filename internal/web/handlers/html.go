// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/rs/zerolog/log"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/binding"
	"github.com/PapagoLabs/outtake/internal/config"
	"github.com/PapagoLabs/outtake/internal/database"
	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/queue"
	"github.com/PapagoLabs/outtake/internal/web/middleware"
	"github.com/PapagoLabs/outtake/internal/web/pages"
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
	return &HTMLHandler{
		queue:    jobQueue,
		db:       db,
		bind:     bind,
		cfg:      cfg,
		product:  product,
		clientID: clientID,
	}
}

// ClipRow renders a single clip card for HTMX polling.
func (handler *HTMLHandler) ClipRow(ctx fiber.Ctx) error {
	id := ctx.Params(paramID)
	job := handler.queue.GetJob(id)
	if job == nil {
		stored, err := handler.db.GetClip(ctx.Context(), id)
		if err != nil {
			return sendStatusCode(ctx, fiber.StatusNotFound)
		}

		job = stored
	}

	return renderHTML(ctx, func(writer io.Writer) error {
		return pages.ClipCard(toClipItem(job)).Render(ctx.Context(), writer)
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
	libraryID := ctx.Query("library")
	parentID := ctx.Query("parent")
	items, libraries := handler.mediaContent(ctx, query, libraryID, parentID)
	props := pages.MediaProps{
		Items:     items,
		Libraries: libraries,
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
			return pages.MediaResults(props).Render(ctx.Context(), writer)
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
		ID:        id,
		Title:     id,
		Type:      "",
		Duration:  0,
		MaxDur:    maxDur,
		Clips:     clips,
		Error:     mediaItemError(itemErr, ctx.Query(queryError)),
		PreviewID: ctx.Query("preview"),
		StartTime: start,
		EndTime:   end,
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
	return renderHTML(ctx, func(writer io.Writer) error {
		return pages.NavLibraries(handler.sidebarLibraries(ctx)).Render(ctx.Context(), writer)
	})
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
			MediaID:    ctx.Query("mediaId"),
			MediaTitle: ctx.Query("title"),
			MediaType:  ctx.Query("type"),
			StartTime:  start,
			Duration:   duration,
			MaxDur:     maxDur,
		}).Render(ctx.Context(), writer)
	})
}

// Playback renders live Plex playback for a media item.
func (handler *HTMLHandler) Playback(ctx fiber.Ctx) error {
	mediaID := ctx.Params(paramID)
	props := pages.PlaybackProps{
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
		return pages.PlaybackPanel(props).Render(ctx.Context(), writer)
	})
}

// PreviewFile serves a generated segment preview.
func (handler *HTMLHandler) PreviewFile(ctx fiber.Ctx) error {
	id := ctx.Params(paramID)
	path := filepath.Join(handler.cfg.StoragePath, "previews", id+".mp4")

	err := ctx.SendFile(path)
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
func (handler *HTMLHandler) clipItems(ctx fiber.Ctx) []pages.ClipItem {
	jobs := handler.listJobs(ctx)
	items := make([]pages.ClipItem, 0, len(jobs))

	for _, job := range jobs {
		items = append(items, toClipItem(job))
	}

	return items
}

// clipsForMedia returns clip cards for one media id.
func (handler *HTMLHandler) clipsForMedia(ctx fiber.Ctx, mediaID string) []pages.ClipItem {
	jobs, err := handler.db.ListClipsForMedia(ctx.Context(), mediaID)
	if err != nil {
		return nil
	}

	items := make([]pages.ClipItem, 0, len(jobs))
	for _, job := range jobs {
		items = append(items, toClipItem(job))
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

// mediaContent loads libraries or media for the media page.
func (handler *HTMLHandler) mediaContent(
	ctx fiber.Ctx,
	query, libraryID, parentID string,
) ([]pages.MediaItem, []pages.LibraryItem) {
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
		return nil, plex.Server{}, false
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
func (handler *HTMLHandler) sidebarLibraries(ctx fiber.Ctx) []pages.LibraryItem {
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

// toClipItem maps a job onto a clips-page card.
func toClipItem(job *queue.Job) pages.ClipItem {
	return pages.ClipItem{
		ID:         job.ID,
		Name:       job.Name,
		MediaID:    job.MediaID,
		MediaTitle: job.MediaTitle,
		ClipType:   string(job.Type),
		Status:     string(job.Status),
		Progress:   job.Progress,
		CreatedAt:  job.CreatedAt.Format(time.RFC3339),
		StartTime:  job.StartTime,
		Duration:   job.Duration,
	}
}

// toLibraryItems maps Plex libraries onto page models.
func toLibraryItems(libs []plex.Library) []pages.LibraryItem {
	out := make([]pages.LibraryItem, 0, len(libs))
	for _, lib := range libs {
		out = append(out, pages.LibraryItem{
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
func mediaCrumbs(libs []pages.LibraryItem, libraryID, upID, upTitle, title string) []pages.Crumb {
	crumbs := []pages.Crumb{{Title: "Libraries", URL: "/media"}}
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

	crumbs = append(crumbs, pages.Crumb{Title: libTitle, URL: libURL})

	if upID != "" {
		upURL := libURL + "&parent=" + url.QueryEscape(
			upID,
		) + "&" + queryTitle + "=" + url.QueryEscape(
			upTitle,
		)

		crumbs = append(crumbs, pages.Crumb{Title: upTitle, URL: upURL})
	}

	if title != "" {
		crumbs = append(crumbs, pages.Crumb{Title: title, URL: ""})
	}

	return crumbs
}

// browseURL builds a drill-down link for a container item.
func browseURL(libraryID, itemID, itemTitle, parentID, parentTitle string) string {
	values := url.Values{}
	values.Set("library", libraryID)
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
) []pages.MediaItem {
	out := make([]pages.MediaItem, 0, len(items))
	for _, item := range items {
		out = append(out, pages.MediaItem{
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

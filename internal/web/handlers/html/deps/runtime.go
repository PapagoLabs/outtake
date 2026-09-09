// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package deps

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	fiber "github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/rs/zerolog/log"

	"github.com/PapagoLabs/outtake/internal/clip/queue"
	"github.com/PapagoLabs/outtake/internal/database"
	"github.com/PapagoLabs/outtake/internal/media"
	mediaquality "github.com/PapagoLabs/outtake/internal/media/quality"
	"github.com/PapagoLabs/outtake/internal/plex"
	plexpage "github.com/PapagoLabs/outtake/internal/plex/page"
	plexserver "github.com/PapagoLabs/outtake/internal/plex/server"
	plextitle "github.com/PapagoLabs/outtake/internal/plex/title"
	"github.com/PapagoLabs/outtake/internal/web/components/browse"
	clipapi "github.com/PapagoLabs/outtake/internal/web/handlers/api/clip"
	sharedplex "github.com/PapagoLabs/outtake/internal/web/handlers/shared/plex"
	"github.com/PapagoLabs/outtake/internal/web/handlers/shared/respond"
	"github.com/PapagoLabs/outtake/internal/web/middleware"
	"github.com/PapagoLabs/outtake/internal/web/pages/dashboard"
	mediapage "github.com/PapagoLabs/outtake/internal/web/pages/media"
	"github.com/PapagoLabs/outtake/internal/web/pages/settings"
	viewclip "github.com/PapagoLabs/outtake/internal/web/view/clip"
	viewmedia "github.com/PapagoLabs/outtake/internal/web/view/media"
)

const (
	// ParamID is the path parameter name for resource identifiers.
	ParamID = "id"
)

const (
	FormChecked       = "1"
	maxProfileNameLen = 64
)

var (
	errNoPlexServer      = errors.New("no plex server selected")
	errProfileName       = errors.New("name is required")
	errProfileNameLength = fmt.Errorf("name must be %d characters or fewer", maxProfileNameLen)
	errProfileCRF        = fmt.Errorf("crf must be between %d and %d", mediaquality.MinCRF, mediaquality.MaxCRF)
	errProfilePreset     = errors.New("unknown encoder preset")
	errProfileAudio      = fmt.Errorf(
		"audio bitrate must be between %d and %d kbps",
		mediaquality.MinAudioKbps,
		mediaquality.MaxAudioKbps,
	)
	errProfileWidth = errors.New("max resolution must be 720p, 1080p, 1440p, or 4K")
)

// Runtime is the shared HTML handler helper surface for feature packages.
type Runtime struct {
	Deps
	AddedAt addedAtIndexCache
}

type DashStats struct {
	Total     int
	Pending   int
	Completed int
	Failed    int
}

// MediaListQuery is the media library browse query state.
type MediaListQuery struct {
	Query     string
	LibraryID string
	ParentID  string
	Sort      string
	Letter    string
	Start     int
	Before    int
}

// MediaListWindow is the PMS container offset and page size for a query.
type MediaListWindow struct {
	Start int
	Size  int
}

// addedAtIndexCache stores added-at jump buckets by server, library, and sort.
type addedAtIndexCache struct {
	mu      sync.Mutex
	indexes map[string][]plex.LetterIndex
}

const (
	// QueryLetter is the media library first-character jump parameter.
	queryQ = "q"

	// querySort is the media library sort parameter.
	querySort = "sort"

	// queryLetter is the media library first-character jump parameter.
	queryLetter = "letter"

	// queryBefore is the exclusive end offset when prepending a previous page.
	queryBefore = "before"

	// addedIndexPageSize is the PMS page size used to build added-at buckets.
	addedIndexPageSize = 200
	// maxAddedIndexPages caps added-at jump-rail collection.
	maxAddedIndexPages = 10

	// maxJumpLabels is the target number of marks on the jump rail.
	maxJumpLabels = 28

	// yearTickDenseMax is the year-count cutoff for five-year ticks.
	yearTickDenseMax = 70
	// yearTickDenseStep keeps every fifth year on a moderately long rail.
	yearTickDenseStep = 5
	// yearTickSparseStep keeps every tenth year on a long rail.
	yearTickSparseStep = 10
	// monthYearLen is the trailing year length in mm/yyyy labels.
	monthYearLen = 4
	// jumpSampleEnds is the first and last marks always kept when sampling.
	jumpSampleEnds = 2
	// jumpOtherKey is the catch-all jump title for unknown letters, years, or dates.
	jumpOtherKey = "#"

	// mediaSortTitleAsc lists titles A-Z.
	mediaSortTitleAsc = "title_asc"
	// mediaSortTitleDesc lists titles Z-A.
	mediaSortTitleDesc = "title_desc"
	// mediaSortYearDesc lists newest years first.
	mediaSortYearDesc = "year_desc"
	// mediaSortYearAsc lists oldest years first.
	mediaSortYearAsc = "year_asc"
	// mediaSortAddedDesc lists recently added titles first.
	mediaSortAddedDesc = "added_desc"
	// mediaSortAddedAsc lists oldest added titles first.
	mediaSortAddedAsc = "added_asc"
)

// MediaPageProps builds library browse props for the current request.
//
// Parameters:
//   - ctx: HTTP request context.
//   - query: Normalized media list query.
//
// Returns:
//   - props: Library browse page props.
func (rt *Runtime) MediaPageProps(
	ctx fiber.Ctx,
	query MediaListQuery,
) viewmedia.MediaProps {
	var letters []plex.LetterIndex

	if !WantsMediaMore(ctx) && !WantsMediaPrev(ctx) {
		letters = rt.MediaLetters(ctx, query)
	}

	Window := query.Window(letters)
	_, _, hasServer := rt.PlexPair()
	items, libraries, total := rt.MediaContent(ctx, query, Window.Start, Window.Size)

	return viewmedia.MediaProps{
		Items:     items,
		Libraries: ChooserLibraries(libraries, query.Query, query.LibraryID),
		Crumbs: MediaCrumbs(
			libraries,
			query.LibraryID,
			ctx.Query(respond.QueryUp),
			ctx.Query(respond.QueryUpTitle),
			ctx.Query(respond.QueryTitle),
		),
		Letters:     ThinJumpIndexes(ToLetterIndexes(letters), query.Sort),
		Query:       query.Query,
		LibraryID:   query.LibraryID,
		ParentID:    query.ParentID,
		ParentTitle: ctx.Query(respond.QueryTitle),
		UpID:        ctx.Query(respond.QueryUp),
		UpTitle:     ctx.Query(respond.QueryUpTitle),
		Sort:        query.Sort,
		Letter:      query.Letter,
		Start:       Window.Start,
		Total:       total,
		PageSize:    respond.MediaPageSize,
		HasServer:   hasServer,
	}
}

// RenderMediaPage writes the media library page or an HTMX fragment.
//
// Parameters:
//   - ctx: HTTP request context.
//   - props: Media library page model.
//
// Returns:
//   - err: The error, if any.
func RenderMediaPage(ctx fiber.Ctx, props *viewmedia.MediaProps) error {
	switch {
	case WantsMediaPrev(ctx):
		return respond.RenderHTML(ctx, func(writer io.Writer) error {
			return browse.MediaPrev(*props).Render(ctx.Context(), writer)
		})
	case WantsMediaMore(ctx):
		return respond.RenderHTML(ctx, func(writer io.Writer) error {
			return browse.MediaMore(*props).Render(ctx.Context(), writer)
		})
	case WantsMediaResults(ctx):
		return respond.RenderHTML(ctx, func(writer io.Writer) error {
			return browse.MediaBrowse(*props).Render(ctx.Context(), writer)
		})
	default:
		return respond.RenderHTML(ctx, func(writer io.Writer) error {
			return mediapage.Media(*props).Render(ctx.Context(), writer)
		})
	}
}

// PreviewWebSafeColor reports whether the preview should use web-safe color.
//
// Prefers the preview redirect query over the config default.
//
// Parameters:
//   - ctx: HTTP request context.
//   - fallback: Config default when the query omits web-safe color.
//
// Returns:
//   - ok: True when the preview should use web-safe color.
func PreviewWebSafeColor(ctx fiber.Ctx, fallback bool) bool {
	raw := ctx.Query(respond.QueryWebSafeColor)
	if raw == "" {
		return fallback
	}

	return raw == FormChecked
}

// WantsMediaResults reports whether the request should swap the media browse pane.
//
// Parameters:
//   - ctx: HTTP request context.
//
// Returns:
//   - ok: True when HX-Target is the media browse pane.
func WantsMediaResults(ctx fiber.Ctx) bool {
	return respond.HxTargetID(ctx.Get(respond.HeaderHXTarget)) == "media-browse"
}

// WantsMediaMore reports whether the request should append the next poster
// page.
//
// Parameters:
//   - ctx: HTTP request context.
//
// Returns:
//   - ok: True when the request should append the next poster page.
func WantsMediaMore(ctx fiber.Ctx) bool {
	return respond.HxTargetID(ctx.Get(respond.HeaderHXTarget)) == "media-more"
}

// WantsMediaPrev reports whether the request should prepend the previous poster
// page.
//
// Parameters:
//   - ctx: HTTP request context.
//
// Returns:
//   - ok: True when the request should prepend the previous poster page.
func WantsMediaPrev(ctx fiber.Ctx) bool {
	return respond.HxTargetID(ctx.Get(respond.HeaderHXTarget)) == "media-prev"
}

// WantsClipList reports whether the request should swap the clips list only.
//
// Parameters:
//   - ctx: HTTP request context.
//
// Returns:
//   - ok: True when the request should swap the clips list only.
func WantsClipList(ctx fiber.Ctx) bool {
	return respond.HxTargetID(ctx.Get(respond.HeaderHXTarget)) == "clip-list"
}

// SelectedLibraryID returns the selected library id.
//
// Parameters:
//   - currentURL: Current url.
//   - fromQuery: From query.
//
// Returns:
//   - id: The selected library id.
func SelectedLibraryID(currentURL, fromQuery string) string {
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

// MediaItemLocation returns the media item location.
//
// Parameters:
//   - mediaID: Media id.
//   - values: Values.
//
// Returns:
//   - value: The media item location.
func MediaItemLocation(mediaID string, values url.Values) string {
	location := "/media/item/" + url.PathEscape(mediaID)
	if encoded := values.Encode(); encoded != "" {
		location += "?" + encoded
	}

	return location
}

// BindSelectedURL sets the binding base URL from the selected server.
//
// Parameters:
//   - ctx: HTTP request context.
//   - rawURL: Raw url.
//
// Returns:
//   - err: The error, if any.
func (rt *Runtime) BindSelectedURL(ctx fiber.Ctx, rawURL string) error {
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
	rt.Bind.Set(server)

	err := rt.DB.SaveSelectedServer(ctx.Context(), server)
	if err != nil {
		log.Warn().Err(err).Msg("failed to persist selected server")
	}

	return respond.RedirectTo(ctx, respond.PathRoot)
}

// ClipMaxDur returns the clip max dur.
//
// Returns:
//   - n: The clip max dur.
func (rt *Runtime) ClipMaxDur() int {
	return rt.ClipMaxDur()
}

// ClipsForMedia returns the clips for media.
//
// Parameters:
//   - ctx: HTTP request context.
//   - mediaID: Media id.
//
// Returns:
//   - items: The clips for media.
func (rt *Runtime) ClipsForMedia(ctx fiber.Ctx, mediaID string) []viewclip.ClipItem {
	jobs, err := rt.DB.ListClipsForMedia(ctx.Context(), mediaID)
	if err != nil {
		return nil
	}

	return rt.JobsToClipItems(ctx, clipapi.ApplyListQuery(jobs, clipapi.ParseListQuery(ctx)))
}

// DiscoverServers returns the discover servers.
//
// Parameters:
//   - ctx: HTTP request context.
//
// Returns:
//   - items: The discover servers.
func (rt *Runtime) DiscoverServers(ctx fiber.Ctx) []plex.Server {
	token := respond.SessionString(session.FromContext(ctx), middleware.SessionKeyToken)
	if token == "" {
		return nil
	}

	plexClient := sharedplex.NewBoundClient(rt.Product, rt.ClientID, token)

	servers, err := plexClient.DiscoverServers(ctx.Context())
	if err != nil {
		log.Warn().Err(err).Msg("discover servers failed")

		return nil
	}

	return servers
}

// JobsToClipItems returns the jobs to clip items.
//
// Parameters:
//   - ctx: HTTP request context.
//   - jobs: Jobs.
//
// Returns:
//   - items: The jobs to clip items.
func (rt *Runtime) JobsToClipItems(ctx fiber.Ctx, jobs []*queue.Job) []viewclip.ClipItem {
	items := make([]viewclip.ClipItem, 0, len(jobs))

	for _, job := range jobs {
		items = append(
			items,
			ToClipItem(job, rt.ClipProfileOptions(ctx), rt.ClipMaxDur()),
		)
	}

	return items
}

// ListJobs returns the list jobs.
//
// Parameters:
//   - ctx: HTTP request context.
//
// Returns:
//   - items: The list jobs.
func (rt *Runtime) ListJobs(ctx fiber.Ctx) []*queue.Job {
	jobs := rt.Queue.GetAllJobs()
	if len(jobs) > 0 {
		return jobs
	}

	stored, err := rt.DB.ListClips(ctx.Context())
	if err != nil {
		return nil
	}

	return stored
}

// LoadMediaItem handles the HTTP request.
//
// Parameters:
//   - ctx: HTTP request context.
//   - mediaID: Media id.
//
// Returns:
//   - mediaItem: The media item.
//   - err: The error, if any.
func (rt *Runtime) LoadMediaItem(ctx fiber.Ctx, mediaID string) (plex.MediaItem, error) {
	plexClient, server, ok := rt.PlexPair()
	if !ok {
		return plex.MediaItem{}, errNoPlexServer
	}

	item, err := plexClient.GetMediaItem(ctx.Context(), server, mediaID)
	if err != nil {
		return plex.MediaItem{}, fmt.Errorf("load media item: %w", err)
	}

	return *item, nil
}

// LookupClip returns the lookup clip.
//
// Parameters:
//   - ctx: HTTP request context.
//   - id: Identifier.
//
// Returns:
//   - job: The lookup clip.
func (rt *Runtime) LookupClip(ctx fiber.Ctx, id string) *queue.Job {
	job := rt.Queue.GetJob(id)
	if job != nil {
		return job
	}

	stored, err := rt.DB.GetClip(ctx.Context(), id)
	if err != nil {
		return nil
	}

	return stored
}

// MediaAudioTracks returns the media audio tracks.
//
// Parameters:
//   - ctx: HTTP request context.
//   - mediaID: Media id.
//
// Returns:
//   - items: The media audio tracks.
func (rt *Runtime) MediaAudioTracks(
	ctx fiber.Ctx,
	mediaID string,
) []viewclip.AudioTrackOption {
	// Skip probing when the media id is missing.
	if mediaID == "" {
		return nil
	}

	path, err := clipapi.ResolveMediaPath(
		ctx.Context(),
		rt.Cfg,
		rt.Bind,
		rt.Product,
		rt.ClientID,
		mediaID,
	)
	if err != nil {
		return nil
	}

	ffmpeg := media.NewExecFFmpeg(rt.Cfg.FFmpegPath, rt.Cfg.FFprobePath)

	info, err := ffmpeg.Probe(ctx.Context(), path)
	if err != nil {
		return nil
	}

	return AudioTrackOptions(info.AudioTracks)
}

// AudioTrackOptions returns the audio track options.
//
// Parameters:
//   - tracks: Tracks.
//
// Returns:
//   - items: The audio track options.
func AudioTrackOptions(tracks []media.AudioTrack) []viewclip.AudioTrackOption {
	options := make([]viewclip.AudioTrackOption, 0, len(tracks))

	for _, track := range tracks {
		options = append(options, viewclip.AudioTrackOption{
			Index: track.Index,
			Label: AudioTrackLabel(track),
		})
	}

	return options
}

// AudioTrackLabel returns the audio track label.
//
// Parameters:
//   - track: Track.
//
// Returns:
//   - value: The audio track label.
func AudioTrackLabel(track media.AudioTrack) string {
	var parts []string

	if track.Language != "" && track.Language != "und" {
		parts = append(parts, track.Language)
	}

	if track.Codec != "" {
		parts = append(parts, track.Codec)
	}

	if layout := mediaquality.ChannelLayoutName(track.Channels); layout != "" {
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

// ChooserLibraries returns the chooser libraries.
//
// Parameters:
//   - libraries: Libraries.
//   - query: Query.
//   - libraryID: Library id.
//
// Returns:
//   - items: The chooser libraries.
func ChooserLibraries(libraries []viewmedia.LibraryItem, query, libraryID string) []viewmedia.LibraryItem {
	if query != "" || libraryID != "" {
		return nil
	}

	return libraries
}

// MediaContent loads libraries or media for the media page.
//
// Parameters:
//   - ctx: HTTP request context.
//   - query: Query.
//   - start: Start.
//   - size: Size.
//
// Returns:
//   - items: The items.
//   - items2: The items.
//   - n: The n.
func (rt *Runtime) MediaContent(
	ctx fiber.Ctx,
	query MediaListQuery,
	start, size int,
) ([]viewmedia.MediaItem, []viewmedia.LibraryItem, int) {
	plexClient, server, ok := rt.PlexPair()
	if !ok {
		return nil, nil, 0
	}

	if query.Query != "" {
		return SearchMediaContent(ctx, plexClient, server, query.Query, query.LibraryID)
	}

	return ListMediaContent(ctx, plexClient, server, query, start, size)
}

// MediaLetters loads jump-rail buckets for a library root.
//
// Parameters:
//   - ctx: HTTP request context.
//   - query: Query.
//
// Returns:
//   - items: The jump-rail buckets for a library root.
func (rt *Runtime) MediaLetters(
	ctx fiber.Ctx,
	query MediaListQuery,
) []plex.LetterIndex {
	if !query.ShowJumpIndex() {
		return nil
	}

	plexClient, server, ok := rt.PlexPair()
	if !ok {
		return nil
	}

	switch query.Sort {
	case mediaSortAddedDesc, mediaSortAddedAsc:
		return LoadAddedAtIndexes(
			ctx.Context(),
			&rt.AddedAt,
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

		return OrderJumpIndex(index, query.Sort)
	default:
		index, err := plexClient.GetFirstCharacters(ctx.Context(), server, query.LibraryID)
		if err != nil {
			log.Warn().Err(err).Msg("list first characters failed")

			return nil
		}

		return OrderJumpIndex(index, query.Sort)
	}
}

// SearchMediaContent returns the search media content.
//
// Parameters:
//   - ctx: HTTP request context.
//   - plexClient: Plex client.
//   - server: Server.
//   - query: Query.
//   - libraryID: Library id.
//
// Returns:
//   - items: The items.
//   - items2: The items.
//   - n: The n.
func SearchMediaContent(
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

		return ToMediaItems(found, libraryID, "", ""), nil, len(found)
	}

	return ToMediaItems(found, libraryID, "", ""), ToLibraryItems(libs), len(found)
}

// ListMediaContent lists a library, a container, or the library chooser.
//
// Parameters:
//   - ctx: HTTP request context.
//   - plexClient: Plex client.
//   - server: Server.
//   - query: Query.
//   - start: Start.
//   - size: Size.
//
// Returns:
//   - items: The items.
//   - items2: The items.
//   - n: The n.
func ListMediaContent(
	ctx fiber.Ctx,
	plexClient *plex.Client,
	server plex.Server,
	query MediaListQuery,
	start, size int,
) ([]viewmedia.MediaItem, []viewmedia.LibraryItem, int) {
	libs, err := plexClient.GetLibraries(ctx.Context(), server)
	if err != nil {
		log.Warn().Err(err).Msg("list libraries failed")

		return nil, nil, 0
	}

	if query.LibraryID == "" {
		return nil, ToLibraryItems(libs), 0
	}

	page, listErr := ListMediaPage(ctx, plexClient, server, query, start, size)
	if listErr != nil {
		log.Warn().Err(listErr).Msg("list media failed")

		return nil, ToLibraryItems(libs), 0
	}

	return ToMediaItems(
			page.Items,
			query.LibraryID,
			query.ParentID,
			ctx.Query(respond.QueryTitle),
		), ToLibraryItems(
			libs,
		), page.Total
}

// PlexPair returns the plex pair.
//
// Returns:
//   - client: The client.
//   - server: The server.
//   - ok: True when the condition holds.
func (rt *Runtime) PlexPair() (*plex.Client, plex.Server, bool) {
	server, ok := rt.Bind.Get()
	if !ok {
		return nil, plex.EmptyServer(), false
	}

	return sharedplex.NewBoundClient(rt.Product, rt.ClientID, server.Token), server, true
}

// SessionItems returns the session items.
//
// Returns:
//   - items: The session items.
func (rt *Runtime) SessionItems() []dashboard.SessionItem {
	sessions := rt.Bind.Sessions()
	items := make([]dashboard.SessionItem, 0, len(sessions))

	for index := range sessions {
		sess := &sessions[index]

		parts, year := SessionTitleParts(sess.MediaItem)

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

// SidebarLibraries returns the sidebar libraries.
//
// Parameters:
//   - ctx: HTTP request context.
//
// Returns:
//   - items: The sidebar libraries.
func (rt *Runtime) SidebarLibraries(ctx fiber.Ctx) []viewmedia.LibraryItem {
	plexClient, server, ok := rt.PlexPair()
	if !ok {
		return nil
	}

	libs, err := plexClient.GetLibraries(ctx.Context(), server)
	if err != nil {
		return nil
	}

	return ToLibraryItems(libs)
}

// ClipStats returns the clip stats.
//
// Parameters:
//   - jobs: Jobs.
//
// Returns:
//   - dashStats: The clip stats.
func ClipStats(jobs []*queue.Job) DashStats {
	stats := DashStats{
		Total:     len(jobs),
		Pending:   0,
		Completed: 0,
		Failed:    0,
	}

	for _, job := range jobs {
		switch job.Status {
		case queue.JobStatusPending, queue.JobStatusProcessing:
			stats.Pending++
		case queue.JobStatusCompleted:
			stats.Completed++
		case queue.JobStatusFailed:
			stats.Failed++
		default:
		}
	}

	return stats
}

// ConnectionURL returns the connection url.
//
// Parameters:
//   - scheme: Scheme.
//   - address: Address.
//   - port: Port.
//
// Returns:
//   - url: The connection url.
func ConnectionURL(scheme, address, port string) string {
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

// ClipFileExists reports whether clip file exists.
//
// Parameters:
//   - path: Filesystem path.
//
// Returns:
//   - ok: True when clip file exists.
func ClipFileExists(path string) bool {
	if path == "" {
		return false
	}

	_, err := os.Stat(path)

	return err == nil
}

// ClipProfileName returns the clip profile name.
//
// Parameters:
//   - quality: Quality.
//   - profiles: Profiles.
//
// Returns:
//   - name: The clip profile name.
func ClipProfileName(quality string, profiles []viewclip.ClipProfileOption) string {
	for _, profile := range profiles {
		if profile.ID == quality {
			return profile.Name
		}
	}

	return quality
}

// ToClipItem returns the to clip item.
//
// Parameters:
//   - job: Job.
//   - profiles: Profiles.
//   - maxDur: Max dur.
//
// Returns:
//   - clipItem: The to clip item.
func ToClipItem(job *queue.Job, profiles []viewclip.ClipProfileOption, maxDur int) viewclip.ClipItem {
	return viewclip.ClipItem{
		ID:            job.ID,
		Name:          job.Name,
		MediaID:       job.MediaID,
		MediaTitle:    job.MediaTitle,
		ClipType:      string(job.Type),
		Status:        string(job.Status),
		Progress:      job.Progress,
		CreatedAt:     FormatClipCreated(job.CreatedAt),
		Error:         job.Error,
		StartTime:     job.StartTime,
		Duration:      job.Duration,
		Quality:       job.Quality,
		ProfileName:   ClipProfileName(job.Quality, profiles),
		Profiles:      profiles,
		FileExists:    ClipFileExists(job.OutputPath),
		AudioIndex:    job.AudioIndex,
		AudioTracks:   nil,
		CropBlackBars: job.CropBlackBars,
		WebSafeColor:  job.WebSafeColor,
		Width:         job.Width,
		FPS:           job.FPS,
		MaxDur:        maxDur,
	}
}

// ToLibraryItems returns the to library items.
//
// Parameters:
//   - libs: Libs.
//
// Returns:
//   - items: The to library items.
func ToLibraryItems(libs []plex.Library) []viewmedia.LibraryItem {
	out := make([]viewmedia.LibraryItem, 0, len(libs))
	for _, lib := range libs {
		out = append(out, viewmedia.LibraryItem{
			ID:        lib.ID,
			Title:     lib.Title,
			Type:      lib.Type,
			ThumbPath: ThumbSrc(lib.ThumbPath),
		})
	}

	return out
}

// ListMediaPage loads one page of a library section or container children.
//
// Parameters:
//   - ctx: HTTP request context.
//   - plexClient: Plex client.
//   - server: Server.
//   - query: Query.
//   - start: Start.
//   - size: Size.
//
// Returns:
//   - mediaPage: The one page of a library section or container children.
//   - err: The error, if any.
func ListMediaPage(
	ctx fiber.Ctx,
	plexClient *plex.Client,
	server plex.Server,
	query MediaListQuery,
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
		PlexMediaSort(query.Sort),
	)
	if err != nil {
		return plex.MediaPage{}, fmt.Errorf("list section: %w", err)
	}

	return page, nil
}

// MediaCrumbs returns the media crumbs.
//
// Parameters:
//   - libs: Libs.
//   - libraryID: Library id.
//   - upID: Up id.
//   - upTitle: Up title.
//   - title: Title.
//
// Returns:
//   - items: The media crumbs.
func MediaCrumbs(libs []viewmedia.LibraryItem, libraryID, upID, upTitle, title string) []viewmedia.Crumb {
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

// SessionTitleParts returns the session title parts.
//
// Parameters:
//   - item: Item.
//
// Returns:
//   - items: The session title parts.
//   - n: The n.
func SessionTitleParts(item plex.MediaItem) ([]viewmedia.Crumb, int) {
	if item.Type != plex.TypeEpisode {
		if item.Title == "" {
			return DisplayTitleCrumb(item), 0
		}

		return []viewmedia.Crumb{{Title: item.Title, URL: SessionItemURL(item.ID)}}, item.Year
	}

	var parts []viewmedia.Crumb

	if item.GrandparentTitle != "" {
		parts = append(parts, viewmedia.Crumb{
			Title: item.GrandparentTitle,
			URL: SessionBrowseURL(
				item.LibraryID,
				item.GrandparentID,
				item.GrandparentTitle,
				"",
				"",
			),
		})
	}

	if label := SeasonSessionLabel(item); label != "" {
		parts = append(parts, viewmedia.Crumb{
			Title: label,
			URL: SessionBrowseURL(
				item.LibraryID,
				item.ParentID,
				label,
				item.GrandparentID,
				item.GrandparentTitle,
			),
		})
	}

	if item.Title != "" {
		parts = append(parts, viewmedia.Crumb{Title: item.Title, URL: SessionItemURL(item.ID)})
	}

	if len(parts) == 0 {
		return DisplayTitleCrumb(item), 0
	}

	return parts, 0
}

// DisplayTitleCrumb returns the display title crumb.
//
// Parameters:
//   - item: Item.
//
// Returns:
//   - items: The display title crumb.
func DisplayTitleCrumb(item plex.MediaItem) []viewmedia.Crumb {
	title := plextitle.Display(item)
	if title == "" {
		return nil
	}

	return []viewmedia.Crumb{{Title: title}}
}

// SeasonSessionLabel returns the season session label.
//
// Parameters:
//   - item: Item.
//
// Returns:
//   - value: The season session label.
func SeasonSessionLabel(item plex.MediaItem) string {
	if item.ParentTitle != "" {
		return item.ParentTitle
	}

	if item.ParentIndex > 0 {
		return "Season " + strconv.Itoa(item.ParentIndex)
	}

	return ""
}

// SessionBrowseURL returns the session browse url.
//
// Parameters:
//   - libraryID: Library id.
//   - itemID: Item id.
//   - itemTitle: Item title.
//   - parentID: Parent id.
//   - parentTitle: Parent title.
//
// Returns:
//   - url: The session browse url.
func SessionBrowseURL(libraryID, itemID, itemTitle, parentID, parentTitle string) string {
	if libraryID == "" || itemID == "" {
		return ""
	}

	return BrowseURL(libraryID, itemID, itemTitle, parentID, parentTitle)
}

// SessionItemURL returns the session item url.
//
// Parameters:
//   - id: Identifier.
//
// Returns:
//   - url: The session item url.
func SessionItemURL(id string) string {
	if id == "" {
		return ""
	}

	return MediaItemLocation(id, url.Values{})
}

// BrowseURL returns the browse url.
//
// Parameters:
//   - libraryID: Library id.
//   - itemID: Item id.
//   - itemTitle: Item title.
//   - parentID: Parent id.
//   - parentTitle: Parent title.
//
// Returns:
//   - url: The browse url.
func BrowseURL(libraryID, itemID, itemTitle, parentID, parentTitle string) string {
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

// ThumbSrc returns the thumb src.
//
// Parameters:
//   - path: Filesystem path.
//
// Returns:
//   - value: The thumb src.
func ThumbSrc(path string) string {
	if path == "" || !plex.ValidThumbPath(path) {
		return ""
	}

	return "/thumbs?path=" + url.QueryEscape(path)
}

// ToMediaItems returns the to media items.
//
// Parameters:
//   - items: Items.
//   - libraryID: Library id.
//   - parentID: Parent id.
//   - parentTitle: Parent title.
//
// Returns:
//   - items: The to media items.
func ToMediaItems(
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
			ThumbPath:    ThumbSrc(item.ThumbPath),
			Browsable:    plex.IsContainerType(item.Type),
			BrowseURL:    BrowseURL(libID, item.ID, item.Title, parentID, parentTitle),
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

// ToServerItems returns the to server items.
//
// Parameters:
//   - servers: Servers.
//   - current: Current.
//
// Returns:
//   - items: The to server items.
func ToServerItems(servers []plex.Server, current plex.Server) []settings.ServerItem {
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

// ClipMatchesStatus reports whether clip matches status.
//
// Parameters:
//   - itemStatus: Item status.
//   - want: Want.
//
// Returns:
//   - ok: True when clip matches status.
func ClipMatchesStatus(itemStatus, want string) bool {
	switch want {
	case viewclip.ClipStatusPending:
		return itemStatus == viewclip.ClipStatusPending || itemStatus == viewclip.ClipStatusProcessing
	default:
		return itemStatus == want
	}
}

// FormatClipCreated returns the format clip created.
//
// Parameters:
//   - created: Created.
//
// Returns:
//   - value: The format clip created.
func FormatClipCreated(created time.Time) string {
	if created.IsZero() {
		return ""
	}

	return created.UTC().Format("Jan 2, 2006 3:04 PM")
}

// PageStart returns the page start.
//
// Parameters:
//   - raw: Raw.
//
// Returns:
//   - n: The page start.
func PageStart(raw string) int {
	start, err := strconv.Atoi(raw)
	if err != nil || start < 0 {
		return 0
	}

	return start
}

// ItemCrumbs returns the item crumbs.
//
// Parameters:
//   - item: Item.
//   - libs: Libs.
//
// Returns:
//   - items: The item crumbs.
func ItemCrumbs(item plex.MediaItem, libs []viewmedia.LibraryItem) []viewmedia.Crumb {
	crumbs := []viewmedia.Crumb{{Title: "Libraries", URL: respond.PathMedia}}

	crumbs = AppendLibraryCrumb(crumbs, item, libs)
	crumbs = AppendShowCrumbs(crumbs, item)
	crumbs = append(crumbs, viewmedia.Crumb{Title: item.Title, URL: ""})

	return crumbs
}

// AppendLibraryCrumb returns the append library crumb.
//
// Parameters:
//   - crumbs: Crumbs.
//   - item: Item.
//   - libs: Libs.
//
// Returns:
//   - items: The append library crumb.
func AppendLibraryCrumb(
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

// AppendShowCrumbs returns the append show crumbs.
//
// Parameters:
//   - crumbs: Crumbs.
//   - item: Item.
//
// Returns:
//   - items: The append show crumbs.
func AppendShowCrumbs(crumbs []viewmedia.Crumb, item plex.MediaItem) []viewmedia.Crumb {
	if item.GrandparentID != "" && item.GrandparentTitle != "" {
		crumbs = append(crumbs, viewmedia.Crumb{
			Title: item.GrandparentTitle,
			URL:   BrowseURL(item.LibraryID, item.GrandparentID, item.GrandparentTitle, "", ""),
		})
	}

	if item.Type != plex.TypeEpisode || item.ParentID == "" {
		return AppendSeasonShowCrumb(crumbs, item)
	}

	seasonTitle := item.ParentTitle
	if seasonTitle == "" {
		seasonTitle = "Season"
	}

	return append(crumbs, viewmedia.Crumb{
		Title: seasonTitle,
		URL: BrowseURL(
			item.LibraryID,
			item.ParentID,
			seasonTitle,
			item.GrandparentID,
			item.GrandparentTitle,
		),
	})
}

// AppendSeasonShowCrumb returns the append season show crumb.
//
// Parameters:
//   - crumbs: Crumbs.
//   - item: Item.
//
// Returns:
//   - items: The append season show crumb.
func AppendSeasonShowCrumb(crumbs []viewmedia.Crumb, item plex.MediaItem) []viewmedia.Crumb {
	if item.Type != plex.TypeSeason || item.ParentID == "" || item.ParentTitle == "" {
		return crumbs
	}

	return append(crumbs, viewmedia.Crumb{
		Title: item.ParentTitle,
		URL:   BrowseURL(item.LibraryID, item.ParentID, item.ParentTitle, "", ""),
	})
}

// ParseMediaListQuery reads search, library, parent, sort, letter, start, and before.
//
// Invalid sort values fall back to title A-Z on a library root.
// Nested containers drop sort and letter.
//
// Parameters:
//   - ctx: HTTP request context.
//
// Returns:
//   - query: Normalized browse state.
func ParseMediaListQuery(ctx fiber.Ctx) MediaListQuery {
	return NormalizeMediaListQuery(MediaListQuery{
		Query:     ctx.Query(queryQ),
		LibraryID: ctx.Query(respond.QueryLibrary),
		ParentID:  ctx.Query(respond.QueryParent),
		Sort:      ctx.Query(querySort),
		Letter:    ctx.Query(queryLetter),
		Start:     PageStart(ctx.Query(respond.QueryStart)),
		Before:    PageStart(ctx.Query(queryBefore)),
	})
}

// NormalizeMediaListQuery drops unknown sort values and nested letter jumps.
//
// Parameters:
//   - query: Query.
//
// Returns:
//   - mediaListQuery: The media list query.
func NormalizeMediaListQuery(query MediaListQuery) MediaListQuery {
	if query.ParentID != "" {
		query.Sort = ""
		query.Letter = ""
		query.Before = 0

		return query
	}

	switch query.Sort {
	case mediaSortTitleAsc,
		mediaSortTitleDesc,
		mediaSortYearDesc,
		mediaSortYearAsc,
		mediaSortAddedDesc,
		mediaSortAddedAsc:
	default:
		if query.LibraryID != "" {
			query.Sort = mediaSortTitleAsc
		} else {
			query.Sort = ""
		}
	}

	if query.Query != "" {
		query.Letter = ""
		query.Before = 0
	}

	return query
}

// IsLibraryRoot reports whether the query is a section listing.
//
// Returns:
//   - ok: True when the query is a section listing.
func (query MediaListQuery) IsLibraryRoot() bool {
	return query.LibraryID != "" && query.ParentID == "" && query.Query == ""
}

// ListStart returns the Plex container offset for this query.
//
// A positive start wins over letter so load-more URLs keep their offset.
//
// Parameters:
//   - index: Index.
//
// Returns:
//   - n: The Plex container offset for this query.
func (query MediaListQuery) ListStart(index []plex.LetterIndex) int {
	if query.Start > 0 {
		return query.Start
	}

	if query.Letter == "" {
		return 0
	}

	return plexpage.LetterOffset(index, query.Letter)
}

// ShowJumpIndex reports whether the library root has a jump rail.
//
// Returns:
//   - ok: True when the library root has a jump rail.
func (query MediaListQuery) ShowJumpIndex() bool {
	return query.IsLibraryRoot()
}

// Window returns the PMS offset and page size for this query.
//
// Parameters:
//   - index: Index.
//
// Returns:
//   - mediaListWindow: The PMS offset and page size for this query.
func (query MediaListQuery) Window(index []plex.LetterIndex) MediaListWindow {
	if query.Before > 0 {
		start := max(query.Before-respond.MediaPageSize, 0)

		return MediaListWindow{Start: start, Size: query.Before - start}
	}

	return MediaListWindow{Start: query.ListStart(index), Size: respond.MediaPageSize}
}

// PlexMediaSort maps an Outtake sort key onto a PMS sort value.
//
// Parameters:
//   - sort: Sort.
//
// Returns:
//   - value: An Outtake sort key onto a PMS sort value.
func PlexMediaSort(sort string) string {
	switch sort {
	case mediaSortTitleAsc:
		return "titleSort:asc"
	case mediaSortTitleDesc:
		return "titleSort:desc"
	case mediaSortYearDesc:
		return "year:desc"
	case mediaSortYearAsc:
		return "year:asc"
	case mediaSortAddedDesc:
		return "addedAt:desc"
	case mediaSortAddedAsc:
		return "addedAt:asc"
	default:
		return ""
	}
}

// ToLetterIndexes maps PMS first-character buckets onto page models.
//
// Empty buckets are omitted. Start is the cumulative offset of each letter.
//
// Parameters:
//   - index: Index.
//
// Returns:
//   - items: The PMS first-character buckets onto page models.
func ToLetterIndexes(index []plex.LetterIndex) []viewmedia.LetterIndex {
	letters := make([]viewmedia.LetterIndex, 0, len(index))
	start := 0

	for _, entry := range index {
		if entry.Size > 0 {
			letters = append(letters, viewmedia.LetterIndex{
				Title: entry.Title,
				Size:  entry.Size,
				Start: start,
			})
		}

		start += entry.Size
	}

	return letters
}

// ThinJumpIndexes reduces rail marks to a readable set of nice ticks.
//
// Parameters:
//   - letters: Letters.
//   - sort: Sort.
//
// Returns:
//   - items: The items.
func ThinJumpIndexes(letters []viewmedia.LetterIndex, sort string) []viewmedia.LetterIndex {
	if len(letters) <= maxJumpLabels {
		return letters
	}

	switch sort {
	case mediaSortYearDesc, mediaSortYearAsc:
		return ThinNumericTitles(letters, YearTickStep(len(letters)))
	case mediaSortAddedDesc, mediaSortAddedAsc:
		return ThinMonthTitles(letters)
	default:
		return letters
	}
}

// YearTickStep chooses the year spacing for a crowded jump rail.
//
// Parameters:
//   - n: N.
//
// Returns:
//   - n: The n.
func YearTickStep(n int) int {
	switch {
	case n <= maxJumpLabels:
		return 1
	case n <= yearTickDenseMax:
		return yearTickDenseStep
	default:
		return yearTickSparseStep
	}
}

// ThinNumericTitles keeps first, last, and step-aligned numeric titles.
//
// Parameters:
//   - letters: Letters.
//   - step: Step.
//
// Returns:
//   - items: The items.
func ThinNumericTitles(letters []viewmedia.LetterIndex, step int) []viewmedia.LetterIndex {
	if step <= 1 || len(letters) <= jumpSampleEnds {
		return letters
	}

	out := make([]viewmedia.LetterIndex, 0, maxJumpLabels)
	last := len(letters) - 1

	for i, letter := range letters {
		if KeepNumericTitle(i, last, letter.Title, step) {
			out = append(out, letter)
		}
	}

	if len(out) > maxJumpLabels {
		return SampleJumpIndexes(out, maxJumpLabels)
	}

	return out
}

// KeepNumericTitle reports whether a numeric jump mark should stay on the rail.
//
// Parameters:
//   - index: Index.
//   - last: Last.
//   - title: Title.
//   - step: Step.
//
// Returns:
//   - ok: True when a numeric jump mark should stay on the rail.
func KeepNumericTitle(index, last int, title string, step int) bool {
	if index == 0 || index == last {
		return true
	}

	year, err := strconv.Atoi(title)
	if err != nil {
		return false
	}

	return year%step == 0
}

// ThinMonthTitles keeps one month mark per year, then samples if still long.
//
// Parameters:
//   - letters: Letters.
//
// Returns:
//   - items: The items.
func ThinMonthTitles(letters []viewmedia.LetterIndex) []viewmedia.LetterIndex {
	if len(letters) <= maxJumpLabels {
		return letters
	}

	yearly := make([]viewmedia.LetterIndex, 0)
	lastYear := ""

	for _, letter := range letters {
		year := MonthJumpYear(letter.Title)
		if year == lastYear && len(yearly) > 0 {
			continue
		}

		yearly = append(yearly, letter)
		lastYear = year
	}

	if len(yearly) <= maxJumpLabels {
		return yearly
	}

	return SampleJumpIndexes(yearly, maxJumpLabels)
}

// MonthJumpYear extracts the year from an mm/yyyy jump title.
//
// Parameters:
//   - title: Title.
//
// Returns:
//   - value: The value.
func MonthJumpYear(title string) string {
	if len(title) >= monthYearLen {
		return title[len(title)-monthYearLen:]
	}

	return title
}

// SampleJumpIndexes picks evenly spaced marks including the ends.
//
// Parameters:
//   - letters: Letters.
//   - limit: Limit.
//
// Returns:
//   - items: The items.
func SampleJumpIndexes(letters []viewmedia.LetterIndex, limit int) []viewmedia.LetterIndex {
	if len(letters) <= limit || limit < jumpSampleEnds {
		return letters
	}

	last := len(letters) - 1
	out := []viewmedia.LetterIndex{letters[0]}

	out = AppendInnerJumpMarks(out, letters, limit-jumpSampleEnds, last)

	if out[len(out)-1].Title != letters[last].Title {
		out = append(out, letters[last])
	}

	return out
}

// AppendInnerJumpMarks adds evenly spaced interior marks between the ends.
//
// Parameters:
//   - out: Out.
//   - letters: Letters.
//   - inner: Inner.
//   - last: Last.
//
// Returns:
//   - items: The items.
func AppendInnerJumpMarks(
	out, letters []viewmedia.LetterIndex,
	inner, last int,
) []viewmedia.LetterIndex {
	for i := 1; i <= inner; i++ {
		idx := i * last / (inner + 1)
		if idx <= 0 || idx >= last || out[len(out)-1].Title == letters[idx].Title {
			continue
		}

		out = append(out, letters[idx])
	}

	return out
}

// OrderJumpIndex sorts or reverses buckets to match the active sort.
//
// Parameters:
//   - index: Index.
//   - sort: Sort.
//
// Returns:
//   - items: The items.
func OrderJumpIndex(index []plex.LetterIndex, sort string) []plex.LetterIndex {
	switch sort {
	case mediaSortTitleDesc:
		return plexpage.ReverseIndexes(index)
	case mediaSortYearDesc:
		return plexpage.ReverseIndexes(plexpage.SortYearIndexes(index))
	case mediaSortYearAsc:
		return plexpage.SortYearIndexes(index)
	default:
		return index
	}
}

// AddedMonthKey formats a Plex addedAt unix timestamp as mm/yyyy.
//
// Parameters:
//   - addedAt: Added at.
//
// Returns:
//   - value: The value.
func AddedMonthKey(addedAt int64) string {
	if addedAt <= 0 {
		return jumpOtherKey
	}

	return time.Unix(addedAt, 0).Local().Format("01/2006")
}

// AddedAtIndexes builds mm/yyyy buckets from a sorted library listing.
//
// Parameters:
//   - items: Items.
//
// Returns:
//   - items: The mm/yyyy buckets from a sorted library listing.
func AddedAtIndexes(items []plex.MediaItem) []plex.LetterIndex {
	return GroupIndexes(items, func(item plex.MediaItem) string {
		return AddedMonthKey(item.AddedAt)
	})
}

// LoadAddedAtIndexes returns cached added-at buckets, collecting them on a
// miss.
//
// Parameters:
//   - ctx: Cancellation context.
//   - cache: Cache.
//   - client: Client.
//   - server: Server.
//   - libraryID: Library id.
//   - sort: Sort.
//
// Returns:
//   - items: The cached added-at buckets, collecting them on a miss.
func LoadAddedAtIndexes(
	ctx context.Context,
	cache *addedAtIndexCache,
	client *plex.Client,
	server plex.Server,
	libraryID, sort string,
) []plex.LetterIndex {
	key := AddedAtCacheKey(server, libraryID, sort)
	if index, ok := cache.Get(key); ok {
		return index
	}

	index := AddedAtIndexes(CollectAddedAtItems(ctx, client, server, libraryID, sort))
	cache.Put(key, index)

	return index
}

// AddedAtCacheKey identifies an added-at jump rail by server, library, and
// sort.
//
// Parameters:
//   - server: Server.
//   - libraryID: Library id.
//   - sort: Sort.
//
// Returns:
//   - value: The value.
func AddedAtCacheKey(server plex.Server, libraryID, sort string) string {
	return server.Address + ":" + strconv.Itoa(server.Port) + "|" + libraryID + "|" + sort
}

// Get loads cached added-at jump indexes.
//
// Parameters:
//   - key: Key.
//
// Returns:
//   - items: The cached added-at jump indexes.
//   - ok: True when the condition holds.
func (cache *addedAtIndexCache) Get(key string) ([]plex.LetterIndex, bool) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	index, ok := cache.indexes[key]

	return slices.Clone(index), ok
}

// Put stores added-at jump indexes in the cache.
//
// Parameters:
//   - key: Key.
//   - index: Index.
func (cache *addedAtIndexCache) Put(key string, index []plex.LetterIndex) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	if cache.indexes == nil {
		cache.indexes = make(map[string][]plex.LetterIndex)
	}

	cache.indexes[key] = slices.Clone(index)
}

// YearIndexes builds year buckets from a sorted library listing.
//
// Parameters:
//   - items: Items.
//
// Returns:
//   - items: The year buckets from a sorted library listing.
func YearIndexes(items []plex.MediaItem) []plex.LetterIndex {
	return GroupIndexes(items, func(item plex.MediaItem) string {
		if item.Year <= 0 {
			return jumpOtherKey
		}

		return strconv.Itoa(item.Year)
	})
}

// TitleIndexes builds first-character buckets from a sorted library listing.
//
// Parameters:
//   - items: Items.
//
// Returns:
//   - items: The first-character buckets from a sorted library listing.
func TitleIndexes(items []plex.MediaItem) []plex.LetterIndex {
	return GroupIndexes(items, func(item plex.MediaItem) string {
		return TitleJumpKey(item.TitleSort, item.Title)
	})
}

// TitleJumpKey returns the first-letter jump key for a title.
//
// Parameters:
//   - titleSort: Title sort.
//   - title: Title.
//
// Returns:
//   - value: The first-letter jump key for a title.
func TitleJumpKey(titleSort, title string) string {
	key := strings.TrimSpace(titleSort)
	if key == "" {
		key = strings.TrimSpace(title)
	}

	if key == "" {
		return jumpOtherKey
	}

	first, _ := utf8.DecodeRuneInString(key)
	upper := unicode.ToUpper(first)
	if upper >= 'A' && upper <= 'Z' {
		return string(upper)
	}

	return jumpOtherKey
}

// GroupIndexes collapses consecutive items that share a jump key.
//
// Parameters:
//   - items: Items.
//   - keyFn: Key fn.
//
// Returns:
//   - items: The items.
func GroupIndexes(items []plex.MediaItem, keyFn func(plex.MediaItem) string) []plex.LetterIndex {
	index := make([]plex.LetterIndex, 0)
	last := ""

	for i := range items {
		key := keyFn(items[i])
		if key == last && len(index) > 0 {
			index[len(index)-1].Size++

			continue
		}

		index = append(index, plex.LetterIndex{Title: key, Size: 1})
		last = key
	}

	return index
}

// CollectAddedAtItems pages a sorted library listing for jump-rail grouping.
//
// Parameters:
//   - ctx: Cancellation context.
//   - client: Client.
//   - server: Server.
//   - libraryID: Library id.
//   - sort: Sort.
//
// Returns:
//   - items: The items.
func CollectAddedAtItems(
	ctx context.Context,
	client *plex.Client,
	server plex.Server,
	libraryID, sort string,
) []plex.MediaItem {
	items := make([]plex.MediaItem, 0)
	start := 0
	pages := 0

	for {
		page, err := client.GetMediaPage(
			ctx,
			server,
			libraryID,
			start,
			addedIndexPageSize,
			PlexMediaSort(sort),
		)
		if err != nil {
			return items
		}

		items = append(items, page.Items...)

		start += len(page.Items)
		pages++

		if len(page.Items) == 0 || start >= page.Total || pages >= maxAddedIndexPages {
			return items
		}
	}
}

// StoredClipProfiles returns the stored clip profiles.
//
// Parameters:
//   - ctx: HTTP request context.
//
// Returns:
//   - items: The stored clip profiles.
func (rt *Runtime) StoredClipProfiles(ctx fiber.Ctx) []database.ClipProfile {
	profiles, err := rt.DB.ListClipProfiles(ctx.Context())
	if err != nil {
		return nil
	}

	return profiles
}

// ClipProfileOptions returns the clip profile options.
//
// Parameters:
//   - ctx: HTTP request context.
//
// Returns:
//   - items: The clip profile options.
func (rt *Runtime) ClipProfileOptions(ctx fiber.Ctx) []viewclip.ClipProfileOption {
	profiles := rt.StoredClipProfiles(ctx)
	options := make([]viewclip.ClipProfileOption, 0, len(profiles))

	for i := range profiles {
		profile := profiles[i]

		options = append(options, viewclip.ClipProfileOption{
			ID:        profile.ID,
			Name:      profile.Name,
			IsDefault: profile.IsDefault,
		})
	}

	if len(options) == 0 {
		return BuiltinProfileOptions()
	}

	return options
}

// BuiltinProfileOptions returns the builtin profile options.
//
// Returns:
//   - items: The builtin profile options.
func BuiltinProfileOptions() []viewclip.ClipProfileOption {
	return []viewclip.ClipProfileOption{
		{ID: string(mediaquality.ClipQualityLow), Name: "Low", IsDefault: false},
		{ID: string(mediaquality.ClipQualityMedium), Name: "Medium", IsDefault: true},
		{ID: string(mediaquality.ClipQualityHigh), Name: "High", IsDefault: false},
	}
}

// ParseClipProfileForm handles the HTTP request.
//
// Parameters:
//   - ctx: HTTP request context.
//   - id: Identifier.
//
// Returns:
//   - clipProfile: The clip profile.
//   - err: The error, if any.
func ParseClipProfileForm(ctx fiber.Ctx, id string) (database.ClipProfile, error) {
	profile, err := ClipProfileFromFields(
		id,
		ctx.FormValue("name"),
		ctx.FormValue("crf"),
		ctx.FormValue("preset"),
		ctx.FormValue("audioKbps"),
		ctx.FormValue("maxWidth"),
	)
	if err != nil {
		return database.ClipProfile{}, fmt.Errorf("parse clip profile: %w", err)
	}

	profile.IsDefault = ctx.FormValue("isDefault") == "1"

	return profile, nil
}

// ClipProfileFromFields returns the clip profile from fields.
//
// Parameters:
//   - id: Identifier.
//   - name: Name.
//   - crfRaw: Crf raw.
//   - preset: Preset.
//   - audioRaw: Audio raw.
//   - widthRaw: Width raw.
//
// Returns:
//   - clipProfile: The clip profile from fields.
//   - err: The error, if any.
func ClipProfileFromFields(
	id, name, crfRaw, preset, audioRaw, widthRaw string,
) (database.ClipProfile, error) {
	// name is required and length-capped.
	name = strings.TrimSpace(name)
	if name == "" {
		return database.ClipProfile{}, errProfileName
	}

	if len(name) > maxProfileNameLen {
		return database.ClipProfile{}, errProfileNameLength
	}

	crf, ok := ParseProfileInt(crfRaw, mediaquality.ValidCRF)
	if !ok {
		return database.ClipProfile{}, errProfileCRF
	}

	if !mediaquality.ValidEncoderPreset(preset) {
		return database.ClipProfile{}, errProfilePreset
	}

	audioKbps, ok := ParseProfileInt(audioRaw, mediaquality.ValidAudioKbps)
	if !ok {
		return database.ClipProfile{}, errProfileAudio
	}

	maxWidth, ok := ParseProfileInt(widthRaw, mediaquality.ValidOutputWidth)
	if !ok {
		return database.ClipProfile{}, errProfileWidth
	}

	now := time.Now().UTC()

	return database.ClipProfile{
		ID:        id,
		Name:      name,
		CRF:       crf,
		Preset:    preset,
		AudioKbps: audioKbps,
		MaxWidth:  maxWidth,
		IsDefault: false,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// ParseProfileInt returns the parse profile int.
//
// Parameters:
//   - raw: Raw.
//   - valid: Valid.
//
// Returns:
//   - n: The parse profile int.
//   - ok: True when the condition holds.
func ParseProfileInt(raw string, valid func(int) bool) (int, bool) {
	value, err := strconv.Atoi(raw)
	if err != nil || !valid(value) {
		return 0, false
	}

	return value, true
}

// ToClipProfileItems returns the to clip profile items.
//
// Parameters:
//   - profiles: Profiles.
//
// Returns:
//   - items: The to clip profile items.
func ToClipProfileItems(profiles []database.ClipProfile) []settings.ClipProfileItem {
	items := make([]settings.ClipProfileItem, 0, len(profiles))

	for i := range profiles {
		profile := profiles[i]

		items = append(items, settings.ClipProfileItem{
			ID:        profile.ID,
			Name:      profile.Name,
			CRF:       profile.CRF,
			Preset:    profile.Preset,
			AudioKbps: profile.AudioKbps,
			MaxWidth:  profile.MaxWidth,
			IsDefault: profile.IsDefault,
		})
	}

	return items
}

// OutputWidthOptions returns the output width options.
//
// Returns:
//   - items: The output width options.
func OutputWidthOptions() []settings.OutputWidthOption {
	options := make([]settings.OutputWidthOption, 0, len(mediaquality.OutputWidths))

	for _, width := range mediaquality.OutputWidths {
		options = append(options, settings.OutputWidthOption{
			Width: width,
			Label: mediaquality.OutputWidthLabel(width),
		})
	}

	return options
}

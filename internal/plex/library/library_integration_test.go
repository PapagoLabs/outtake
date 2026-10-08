// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package library_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/ffmpeg/probe"
	"github.com/PapagoLabs/outtake/internal/plex"
	"github.com/PapagoLabs/outtake/internal/plex/identity"
	"github.com/PapagoLabs/outtake/internal/plex/library"
	"github.com/PapagoLabs/outtake/internal/plex/library/mocks"
	"github.com/PapagoLabs/outtake/internal/settings/config"
)

// fakeServer is a loopback stand-in for a Plex Media Server.
//
// Parameters:
//   - t: The test that needs the server.
//
// Returns:
//   - server: The running fake.
//   - requests: Every path the fake was asked for, in order.
func fakeServer(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()

	var mu sync.Mutex

	seen := make([]string, 0)

	payloads := map[string]string{
		"/library/sections/all": `{"MediaContainer":{"Directory":[
			{"key":"1","title":"Movies","type":"movie"},
			{"key":"2","title":"TV Shows","type":"show"}
		]}}`,
		"/library/sections/1/all": `{"MediaContainer":{"totalSize":2,"Metadata":[
			{"ratingKey":"100","title":"Alien","type":"movie","year":1979,
			 "duration":6900000,"librarySectionID":"1"},
			{"ratingKey":"101","title":"Aliens","type":"movie","year":1986,
			 "duration":8220000,"librarySectionID":"1"}
		]}}`,
		"/library/sections/1/firstCharacter": `{"MediaContainer":{"Directory":[
			{"key":"A","title":"A","size":2},
			{"key":"B","title":"B","size":1},
			{"key":"#","title":"#","size":0}
		]}}`,
		"/library/sections/1/year": `{"MediaContainer":{"Directory":[
			{"key":"1986","title":"1986","size":1},
			{"key":"1979","title":"1979","size":1}
		]}}`,
		"/library/metadata/10/children": `{"MediaContainer":{"totalSize":2,"Metadata":[
			{"ratingKey":"11","title":"Season 1","type":"season","index":1},
			{"ratingKey":"12","title":"Season 2","type":"season","index":2}
		]}}`,
		"/library/metadata/100": `{"MediaContainer":{"Metadata":[
			{"ratingKey":"100","title":"Alien","type":"movie",
			 "Media":[{"Part":[{"file":"/plex-media/movies/Alien.mkv"}]}]}
		]}}`,
		"/hubs/search": `{"MediaContainer":{"Hub":[
			{"title":"Movies","type":"movie","Metadata":[
				{"ratingKey":"100","title":"Alien","type":"movie","duration":6900000}
			]}
		]}}`,
		"/status/sessions": `{"MediaContainer":{"size":0}}`,
	}

	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mu.Lock()

		seen = append(seen, request.URL.Path)
		mu.Unlock()

		body, ok := payloads[request.URL.Path]
		if !ok {
			writer.WriteHeader(http.StatusNotFound)

			return
		}

		writer.Header().Set("Content-Type", "application/json")

		_, _ = writer.Write([]byte(body))
	})

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	requested := func() []string {
		mu.Lock()
		defer mu.Unlock()

		return append([]string(nil), seen...)
	}

	return server, requested
}

// boundServer returns a real Plex client and server pair pointed at the fake.
//
// Parameters:
//   - t: The test that needs the pair.
//   - baseURL: Loopback URL the fake is listening on.
//
// Returns:
//   - client: A Plex client scoped to the fake.
//   - server: The Plex server identity the fake is reached as.
func boundServer(baseURL string) (*plex.Client, plex.Server) {
	server, ok := plex.ServerFromURL(baseURL, "test-token")
	if !ok {
		panic("the fake server URL must parse into a Plex server")
	}

	return plex.NewClient(plex.ClientConfig{
		Product:  "outtake",
		ClientID: "test-client",
		Token:    "test-token",
		Timeout:  5 * time.Second,
		BaseURL:  baseURL,
	}), server
}

// boundSelection returns a real identity binding selected onto the fake server.
//
// Parameters:
//   - t: The test that needs the binding.
//   - baseURL: Loopback URL the fake is listening on.
//
// Returns:
//   - bind: The binding, stopped when the test finishes.
func boundSelection(t *testing.T, baseURL string) *identity.Binding {
	t.Helper()

	server, ok := plex.ServerFromURL(baseURL, "test-token")
	require.True(t, ok, "the fake server URL must parse into a Plex server")

	bind := identity.NewBinding("outtake", "test-client", time.Hour)
	bind.Set(server)

	t.Cleanup(bind.Stop)

	return bind
}

// unselected returns a selection with no server behind it.
//
// Parameters:
//   - t: The test that needs the selection.
//
// Returns:
//   - selected: The unbound selection.
func unselected(t *testing.T) *mocks.MockServerSelection {
	t.Helper()

	selected := mocks.NewMockServerSelection(t)
	selected.EXPECT().
		Client().
		Return(nil, plex.EmptyServer(), false).
		Maybe()

	return selected
}

// e2eConfig returns the configuration the e2e environment resolves media ids under.
//
// Parameters:
//   - t: The test that needs the configuration.
//
// Returns:
//   - cfg: A configuration with the e2e environment set.
func e2eConfig(t *testing.T) *config.Config {
	t.Helper()

	return &config.Config{
		Env:             "e2e",
		DatabasePath:    filepath.Join(t.TempDir(), "outtake.db"),
		StoragePath:     filepath.Join(t.TempDir(), "output"),
		StorageBackend:  "filesystem",
		SessionPoll:     10 * time.Second,
		NumWorkers:      1,
		MaxClipDur:      600 * time.Second,
		PlexMediaRoot:   "/plex-media",
		LocalMediaRoot:  t.TempDir(),
		PublicBaseURL:   "",
		ListenAddr:      "127.0.0.1:0",
		DatabaseBackend: "sqlite",
		LogLevel:        "error",
	}
}

// "movies/Alien.mkv"File writes a media file inside the test's temp directory.
//
// Parameters:
//   - t: The test that needs the file.
//
// Returns:
//   - path: The file path, which doubles as a media id under e2e.
func localMediaFile(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "source.mkv")
	require.NoError(t, os.WriteFile(path, []byte("not really a video"), 0o644))

	return path
}

// addedAt returns a mid-month 2024 timestamp for an added-at bucket fixture.
//
// Parameters:
//   - month: Month of the timestamp.
//
// Returns:
//   - seconds: Unix seconds for the middle of that month.
func addedAt(month time.Month) int64 {
	return time.Date(2024, month, 15, 12, 0, 0, 0, time.UTC).Unix()
}

// indexTitles returns the bucket titles of a jump rail in order.
//
// Parameters:
//   - buckets: Jump rail buckets.
//
// Returns:
//   - titles: The bucket titles, in order.
func indexTitles(buckets []plex.LetterIndex) []string {
	titles := make([]string, 0, len(buckets))
	for _, bucket := range buckets {
		titles = append(titles, bucket.Title)
	}

	return titles
}

// hdrProbe is the probe result the source resolver is handed.
//
// Returns:
//   - info: A probe result describing an HDR source with two audio tracks.
func hdrProbe() probe.Info {
	return probe.Info{
		Duration:      2 * time.Hour,
		Width:         3840,
		Height:        2160,
		VideoCodec:    "hevc",
		AudioCodec:    "truehd",
		Format:        "matroska",
		BitRate:       60_000_000,
		ColorTransfer: "smpte2084",
		AudioTracks: []probe.Track{
			{Index: 1, Codec: "truehd", Language: "eng", Title: "Commentary", Channels: 2},
			{Index: 2, Codec: "eac3", Language: "eng", Title: "", Channels: 6},
		},
	}
}

func TestIntegration_E2EMediaIDIsReadAsALocalFile(t *testing.T) {
	t.Parallel()

	cfg := e2eConfig(t)
	path := localMediaFile(t)

	// The selection is never consulted, so a call into it fails the test.
	selected := mocks.NewMockServerSelection(t)

	resolved, err := library.ResolveMediaPath(t.Context(), cfg, selected, path)
	require.NoError(t, err)
	assert.Equal(t, path, resolved)
}

func TestIntegration_LocalFileIsNotAMediaIDOutsideE2E(t *testing.T) {
	t.Parallel()

	cfg := e2eConfig(t)

	cfg.Env = "production"

	_, err := library.ResolveMediaPath(t.Context(), cfg, unselected(t), localMediaFile(t))
	require.ErrorIs(t, err, library.ErrNoServer,
		"outside e2e a path is just an id the server has to resolve")
}

func TestIntegration_ResolveMediaPathRemapsThePlexPathOntoTheLocalMount(t *testing.T) {
	t.Parallel()

	server, requested := fakeServer(t)
	cfg := e2eConfig(t)

	cfg.Env = "production"
	cfg.LocalMediaRoot = filepath.Join(t.TempDir(), "mnt")

	bind := boundSelection(t, server.URL)

	resolved, err := library.ResolveMediaPath(t.Context(), cfg, bind, "100")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(cfg.LocalMediaRoot, "movies/Alien.mkv"), resolved)
	assert.Contains(t, requested(), "/library/metadata/100")
}

func TestIntegration_ResolveMediaPathReportsNoServerWhenNothingIsSelected(t *testing.T) {
	t.Parallel()

	cfg := e2eConfig(t)

	cfg.Env = "production"

	bind := identity.NewBinding("outtake", "test-client", time.Hour)
	t.Cleanup(bind.Stop)

	_, err := library.ResolveMediaPath(t.Context(), cfg, bind, "100")
	require.ErrorIs(t, err, library.ErrNoServer)
}

func TestIntegration_MediaSourceResolvesAndDescribesThroughTheRealBinding(t *testing.T) {
	t.Parallel()

	server, _ := fakeServer(t)
	cfg := e2eConfig(t)

	cfg.Env = "production"

	bind := boundSelection(t, server.URL)
	prober := mocks.NewMockProber(t)
	prober.EXPECT().
		Probe(mock.Anything, mock.Anything).
		Return(hdrProbe(), nil).
		Maybe()

	source := library.NewMediaSource(cfg, bind, prober)

	resolved, err := source.Resolve(t.Context(), "100")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(cfg.LocalMediaRoot, "movies/Alien.mkv"), resolved)

	duration, ok := source.Duration(t.Context(), resolved)
	require.True(t, ok)
	assert.Equal(t, 2*time.Hour, duration)

	described := source.Describe(t.Context(), "100")
	assert.Equal(t, resolved, described.Path)
	assert.Equal(t, 2*time.Hour, described.Duration)
	assert.Equal(t, "smpte2084", described.ColorTransfer)
	assert.True(t, described.HDR)
	assert.Equal(t, "4K HDR10", described.Quality)
	require.Len(t, described.AudioStreams, 2)
	assert.Equal(t, "Stereo", described.AudioStreams[0].Layout)
	assert.Equal(t, 1, described.AudioStreams[0].Index)
	assert.Equal(t, "5.1", described.AudioStreams[1].Layout)

	assert.Equal(t, library.SourceInfo{}, source.Describe(t.Context(), ""))
	assert.Equal(t, library.SourceInfo{}, source.Describe(t.Context(), "absent"))
}

func TestIntegration_DescribePathReportsAProbeFailure(t *testing.T) {
	t.Parallel()

	cfg := e2eConfig(t)
	prober := mocks.NewMockProber(t)
	prober.EXPECT().
		Probe(mock.Anything, mock.Anything).
		Return(probe.Info{}, os.ErrNotExist)

	source := library.NewMediaSource(cfg, unselected(t), prober)

	path := filepath.Join(t.TempDir(), "source.mkv")
	described := source.DescribePath(t.Context(), path)
	assert.Equal(t, path, described.Path, "the path survives an unreadable probe")
	assert.Zero(t, described.Duration)
	assert.Empty(t, described.AudioStreams)

	_, ok := source.Duration(t.Context(), path)
	assert.False(t, ok, "an unreadable source has no usable length")
}

func TestIntegration_BrowseReadsTheLibraryChooserAndASectionPage(t *testing.T) {
	t.Parallel()

	server, requested := fakeServer(t)
	client, pms := boundServer(server.URL)

	chooser, err := library.Browse(
		t.Context(),
		&library.LibraryCache{},
		client,
		pms,
		library.Query{},
		library.Window{},
	)
	require.NoError(t, err)
	require.Len(t, chooser.Libraries, 2)
	assert.Equal(t, "Movies", chooser.Libraries[0].Title)
	assert.Empty(t, chooser.Items, "the chooser lists libraries, not media")
	assert.Zero(t, chooser.Total)

	section, err := library.Browse(
		t.Context(), &library.LibraryCache{},
		client,
		pms,
		library.NormalizeQuery(library.Query{LibraryID: "1", Sort: library.SortYearDesc}),
		library.Window{Start: 0, Size: library.PageSize},
	)
	require.NoError(t, err)
	require.Len(t, section.Items, 2)
	assert.Equal(t, 2, section.Total)
	assert.Equal(t, "Alien", section.Items[0].Title)
	assert.InDelta(t, 6900.0, section.Items[0].Duration, 0.01)

	search, err := library.Browse(
		t.Context(), &library.LibraryCache{},
		client,
		pms,
		library.NormalizeQuery(library.Query{Query: "alien", LibraryID: "1"}),
		library.Window{Start: 0, Size: library.PageSize},
	)
	require.NoError(t, err)
	require.Len(t, search.Items, 1)
	assert.Equal(t, "Alien", search.Items[0].Title)
	assert.Equal(t, 1, search.Total)

	assert.Contains(t, requested(), "/hubs/search")
	assert.Contains(t, requested(), "/library/sections/1/all")
}

func TestIntegration_BrowseReadsContainerChildren(t *testing.T) {
	t.Parallel()

	server, requested := fakeServer(t)
	client, pms := boundServer(server.URL)

	children, err := library.Browse(
		t.Context(), &library.LibraryCache{},
		client,
		pms,
		library.NormalizeQuery(library.Query{LibraryID: "1", ParentID: "10"}),
		library.Window{Start: 0, Size: library.PageSize},
	)
	require.NoError(t, err)
	require.Len(t, children.Items, 2)
	assert.Equal(t, "Season 1", children.Items[0].Title)
	assert.Equal(t, 2, children.Total)

	assert.Contains(t, requested(), "/library/metadata/10/children")
}

func TestIntegration_JumpRailFollowsTheSort(t *testing.T) {
	t.Parallel()

	server, requested := fakeServer(t)
	client, pms := boundServer(server.URL)

	letters, err := library.JumpIndex(
		t.Context(),
		client,
		pms,
		library.NormalizeQuery(library.Query{LibraryID: "1", Sort: library.SortTitleAsc}),
		nil,
	)
	require.NoError(t, err)
	require.Len(t, letters, 3)
	assert.Equal(t, []string{"A", "B", "#"}, indexTitles(letters))

	reversed, err := library.JumpIndex(
		t.Context(),
		client,
		pms,
		library.NormalizeQuery(library.Query{LibraryID: "1", Sort: library.SortTitleDesc}),
		nil,
	)
	require.NoError(t, err)
	require.Len(t, reversed, 3)
	assert.Equal(t, []string{"#", "B", "A"}, indexTitles(reversed),
		"a descending title sort reverses the rail")

	years, err := library.JumpIndex(
		t.Context(),
		client,
		pms,
		library.NormalizeQuery(library.Query{LibraryID: "1", Sort: library.SortYearAsc}),
		nil,
	)
	require.NoError(t, err)
	require.Len(t, years, 2)
	assert.Equal(t, "1979", years[0].Title, "a year rail is ordered oldest first")
	assert.Equal(t, "1986", years[1].Title)

	marks := library.JumpMarks(letters, library.SortTitleAsc)
	require.Len(t, marks, 2, "an empty bucket is not a jump target")
	assert.Equal(t, "A", marks[0].Title)

	none, err := library.JumpIndex(
		t.Context(),
		client,
		pms,
		library.NormalizeQuery(library.Query{Query: "alien"}),
		nil,
	)
	require.NoError(t, err)
	assert.Nil(t, none, "a search has no jump rail")

	assert.Contains(t, requested(), "/library/sections/1/firstCharacter")
}

func TestIntegration_QueryWindowAndLetterJumpArithmetic(t *testing.T) {
	t.Parallel()

	buckets := []plex.LetterIndex{
		{Title: "A", Size: 2},
		{Title: "B", Size: 1},
		{Title: "C", Size: 3},
	}

	letter := library.NormalizeQuery(library.Query{LibraryID: "1", Letter: "B"})
	assert.True(t, letter.IsLibraryRoot())
	assert.True(t, letter.ShowJumpIndex())
	assert.Equal(
		t,
		2,
		letter.ListStart(buckets),
		"a jump lands on the end of the letters before it",
	)
	assert.Equal(t, library.Window{Start: 2, Size: library.PageSize}, letter.Window(buckets))

	capped := library.NormalizeQuery(library.Query{LibraryID: "1", Letter: "C"})
	assert.Equal(t, 3, capped.ListStart(buckets))

	explicit := library.NormalizeQuery(library.Query{LibraryID: "1", Start: 24, Letter: "B"})
	assert.Equal(t, 24, explicit.ListStart(buckets), "an explicit offset beats the letter jump")

	before := library.NormalizeQuery(library.Query{LibraryID: "1", Before: 60})
	assert.Equal(t, library.Window{Start: 12, Size: 48}, before.Window(buckets))

	nested := library.NormalizeQuery(library.Query{
		LibraryID: "1",
		ParentID:  "10",
		Sort:      library.SortTitleDesc,
		Letter:    "B",
		Before:    60,
	})
	assert.Empty(t, nested.Sort)
	assert.Empty(t, nested.Letter)
	assert.Zero(t, nested.Before)
	assert.False(t, nested.IsLibraryRoot())
	assert.False(t, nested.ShowJumpIndex())

	assert.Equal(t, 0, library.ParseStart("not a number"))
	assert.Equal(t, 0, library.ParseStart("-4"))
	assert.Equal(t, 12, library.ParseStart("12"))

	assert.Equal(t, "titleSort:desc", library.PlexSort(library.SortTitleDesc))
	assert.Equal(t, "addedAt:asc", library.PlexSort(library.SortAddedAsc))
	assert.Empty(t, library.PlexSort(nested.Sort), "a nested listing carries no sort")
}

func TestIntegration_IndexesAreBuiltFromASortedListing(t *testing.T) {
	t.Parallel()

	byTitle := []plex.MediaItem{
		{ID: "1", Title: "Alien", TitleSort: "Alien, 1979", Year: 1979},
		{ID: "2", Title: "Aliens", TitleSort: "Aliens, 1986", Year: 1986},
		{ID: "3", Title: "Amityville", TitleSort: "Amityville, 1979", Year: 1979},
		{ID: "4", Title: "The Thing", TitleSort: "Thing, The, 1982", Year: 1982},
		{ID: "5", Title: "#1", Year: 0},
	}

	titles := library.TitleIndexes(byTitle)
	require.Len(t, titles, 3)
	assert.Equal(t, "A", titles[0].Title)
	assert.Equal(t, 3, titles[0].Size)
	assert.Equal(t, "T", titles[1].Title)
	assert.Equal(t, library.JumpOtherKey, titles[2].Title,
		"a title that starts with nothing else falls into the other bucket")

	byYear := []plex.MediaItem{
		{ID: "1", Title: "Alien", Year: 1979, AddedAt: addedAt(time.January)},
		{ID: "2", Title: "Amityville", Year: 1979, AddedAt: addedAt(time.February)},
		{ID: "3", Title: "The Thing", Year: 1982, AddedAt: addedAt(time.March)},
		{ID: "4", Title: "Aliens", Year: 1986, AddedAt: addedAt(time.April)},
		{ID: "5", Title: "Unknown", Year: 0, AddedAt: addedAt(time.May)},
		{ID: "6", Title: "Undated", Year: 0},
	}

	years := library.YearIndexes(byYear)
	require.Len(t, years, 4)
	assert.Equal(t, "1979", years[0].Title)
	assert.Equal(t, 2, years[0].Size, "consecutive items of a year share a bucket")
	assert.Equal(t, "1982", years[1].Title)
	assert.Equal(t, "1986", years[2].Title)
	assert.Equal(
		t,
		library.JumpOtherKey,
		years[3].Title,
		"an item with no year lands in the other bucket",
	)
	assert.Equal(t, 2, years[3].Size)

	months := library.AddedAtIndexes(byYear)
	require.Len(t, months, 6)
	assert.Equal(t, "01/2024", months[0].Title)
	assert.Equal(t, "05/2024", months[4].Title)
	assert.Equal(
		t,
		library.JumpOtherKey,
		months[5].Title,
		"an item never added is not dated at all",
	)

	thinned := library.JumpMarks(months, library.SortAddedAsc)
	assert.Len(t, thinned, 6, "six marks need no thinning")

	assert.Equal(t, "T", library.TitleJumpKey("", "The Thing"))
	assert.Equal(t, "A", library.TitleJumpKey("Amityville, 1979", "ignored"))
	assert.Equal(t, library.JumpOtherKey, library.TitleJumpKey("", "1979"))
}

func TestIntegration_ItemResponsesMapPlexMediaItems(t *testing.T) {
	t.Parallel()

	items := []plex.MediaItem{
		{
			ID: "100", Title: "Alien", Type: "movie", Duration: 6900,
			ThumbPath: "/library/metadata/100/thumb", LibraryTitle: "Movies", Year: 1979,
		},
		{
			ID: "200", Title: "Pilot", Type: "episode", Duration: 2700,
			ThumbPath: "/library/metadata/200/thumb", LibraryTitle: "TV Shows",
			ParentIndex: 1, Index: 1, GrandparentTitle: "Firefly",
		},
	}

	responses := library.ItemResponses(items)
	require.Len(t, responses, 2)

	assert.Equal(t, "Alien (1979)", responses[0].Title, "a movie with a year shows it")
	assert.Equal(t, "Movies", responses[0].LibraryTitle)
	assert.Equal(t, 1979, responses[0].Year)
	assert.Equal(t, "/library/metadata/100/thumb", responses[0].ThumbPath)
	assert.Zero(t, responses[0].Season)
	assert.Empty(t, responses[0].ShowTitle)

	assert.Equal(t, "Firefly · S01E01 · Pilot", responses[1].Title,
		"an episode is labeled with its show, code, and own title")
	assert.Equal(t, "Firefly", responses[1].ShowTitle)
	assert.Equal(t, 1, responses[1].Season)
	assert.Equal(t, 1, responses[1].Episode)
	assert.Equal(t, "S01E01", plex.EpisodeCode(1, 1))

	assert.Empty(t, library.ItemResponses(nil))
}

func TestIntegration_SessionResponsesMapPlexSessions(t *testing.T) {
	t.Parallel()

	sessions := []plex.Session{
		{
			ID:    "session-1",
			Title: "Alien",
			MediaItem: plex.MediaItem{
				ID: "100", Title: "Alien", Type: "movie", Year: 1979,
			},
			Duration:   6900,
			ViewOffset: 120,
		},
		{
			ID:    "session-2",
			Title: "Pilot",
			MediaItem: plex.MediaItem{
				ID: "200", Title: "Pilot", Type: "episode",
			},
			Duration: 2700,
		},
	}

	responses := library.SessionResponses(sessions)
	require.Len(t, responses, 2)

	assert.Equal(t, "session-1", responses[0].ID)
	assert.Equal(t, "100", responses[0].MediaID)
	assert.Equal(
		t,
		"Alien (1979)",
		responses[0].Title,
		"the session title comes from the media item",
	)
	assert.InDelta(t, 6900.0, responses[0].Duration, 0.01)
	assert.InDelta(t, 120.0, responses[0].ViewOffset, 0.01)

	assert.Equal(t, "session-2", responses[1].ID)
	assert.Equal(t, "Pilot", responses[1].Title)
	assert.Zero(t, responses[1].ViewOffset)

	assert.Empty(t, library.SessionResponses(nil))
}

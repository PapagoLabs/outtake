// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package exportform

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/api"
	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/clip/profile"
	"github.com/PapagoLabs/outtake/internal/settings/config"
	"github.com/PapagoLabs/outtake/internal/web/routes"
	"github.com/PapagoLabs/outtake/internal/web/view"
)

// queryForm runs a reader against one GET request and returns what it read.
//
// Parameters:
//   - t: The test the request belongs to.
//   - target: Request target, including any query string.
//   - read: Reader invoked with the parsed request context.
//
// Returns:
//   - got: The value the reader produced.
func queryForm(t *testing.T, target string, read func(fiber.Ctx) any) any {
	t.Helper()

	app := fiber.New()

	var got any

	app.Get(routes.PathMedia, func(ctx fiber.Ctx) error {
		got = read(ctx)

		return ctx.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	return got
}

// fromQueryOf reads the export form off a request target.
//
// Parameters:
//   - t: The test the request belongs to.
//   - target: Request target, including any query string.
//   - defaultCrop: Configured crop-black-bars default.
//   - defaultPreserve: Configured keep-HDR default.
//
// Returns:
//   - form: The export form the target carries.
func fromQueryOf(
	t *testing.T,
	target string,
	defaultCrop, defaultPreserve bool,
) view.ExportForm {
	t.Helper()

	form, ok := queryForm(t, target, func(ctx fiber.Ctx) any {
		return FromQuery(ctx, defaultCrop, defaultPreserve, "The Movie")
	}).(view.ExportForm)
	require.True(t, ok)

	return form
}

// mediaItemFormOf reads the media item export form off a request target.
//
// Parameters:
//   - t: The test the request belongs to.
//   - target: Request target, including any query string.
//   - cfg: Configuration supplying the crop default.
//
// Returns:
//   - form: The export form the target carries, offering the built-in profiles.
func mediaItemFormOf(t *testing.T, target string, cfg *config.Config) view.ExportForm {
	t.Helper()

	form, ok := queryForm(t, target, func(ctx fiber.Ctx) any {
		return MediaItemForm(ctx, cfg, "The Movie", profile.BuiltinProfiles())
	}).(view.ExportForm)
	require.True(t, ok)

	return form
}

// clipWindowOf reads the export form window off a request target.
//
// Parameters:
//   - t: The test the request belongs to.
//   - target: Request target, including any query string.
//
// Returns:
//   - window: The start and end marks the target carries.
func clipWindowOf(t *testing.T, target string) Window {
	t.Helper()

	window, ok := queryForm(t, target, func(ctx fiber.Ctx) any {
		return ClipWindow(ctx)
	}).(Window)
	require.True(t, ok)

	return window
}

// values builds a query string from name and value pairs.
//
// Parameters:
//   - pairs: Alternating query parameter names and values.
//
// Returns:
//   - query: The encoded query string.
func values(pairs ...string) string {
	form := url.Values{}
	for index := 0; index+1 < len(pairs); index += 2 {
		form.Set(pairs[index], pairs[index+1])
	}

	return form.Encode()
}

func TestFromRequestCarriesEveryExportField(t *testing.T) {
	t.Parallel()

	form := FromRequest(api.ClipRequest{
		ClipType:      string(clip.TypeGIF),
		Name:          "Outro",
		Quality:       "archive",
		AudioIndex:    2,
		Width:         480,
		FPS:           15,
		CropBlackBars: true,
		WebSafeColor:  new(true),
		PreserveHDR:   new(true),
	})

	assert.Equal(t, view.ExportForm{
		Type:          clip.TypeGIF,
		Name:          "Outro",
		Quality:       "archive",
		AudioIndex:    2,
		Width:         480,
		FPS:           15,
		CropBlackBars: true,
		PreserveHDR:   true,
	}, form, "preserveHdr wins over the legacy webSafeColor")
}

func TestFromRequestReadsAnAbsentFlagAsOff(t *testing.T) {
	t.Parallel()

	form := FromRequest(api.ClipRequest{MediaID: "42"})

	assert.False(t, form.PreserveHDR,
		"a request that omitted the field did not ask to keep HDR as is")
	assert.Zero(t, form.AudioIndex)
	assert.Zero(t, form.Width)
	assert.Zero(t, form.FPS)
	assert.Empty(t, form.Quality)
}

func TestFromQueryFallsBackToTheMediaTitle(t *testing.T) {
	t.Parallel()

	form := fromQueryOf(t, routes.PathMedia, false, false)

	assert.Equal(t, "The Movie", form.Name,
		"an unvisited form is named after what it would cut")
	assert.Equal(t, clip.TypeClip, form.Type,
		"the form defaults to a plain clip")
}

func TestFromQueryKeepsAClearedName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		give   string
		want   string
		reason string
	}{
		{
			name:   "a carried name replaces the title",
			give:   "Outro",
			want:   "Outro",
			reason: "the user typed a name, so that is what the form shows",
		},
		{
			name:   "an explicitly empty name is not refilled",
			give:   "",
			want:   "",
			reason: "clearing the field has to survive the redirect that carried it",
		},
		{
			name:   "a name of spaces is kept as typed",
			give:   "  ",
			want:   "  ",
			reason: "the form state is carried exactly as it was submitted",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			target := routes.PathMedia + "?" + url.Values{
				routes.QueryExportName: {test.give},
			}.Encode()

			form := fromQueryOf(t, target, false, false)

			assert.Equal(t, test.want, form.Name, test.reason)
		})
	}
}

func TestFromQueryReadsTheEncodingOptions(t *testing.T) {
	t.Parallel()

	target := routes.PathMedia + "?" + values(
		routes.QueryExportType, string(clip.TypeGIF),
		routes.QueryQuality, "archive",
		routes.QueryAudioIndex, "3",
		routes.QueryWidth, "320",
		routes.QueryFPS, "12",
	)

	form := fromQueryOf(t, target, false, false)

	assert.Equal(t, clip.TypeGIF, form.Type)
	assert.Equal(t, "archive", form.Quality)
	assert.Equal(t, 3, form.AudioIndex)
	assert.Equal(t, 320, form.Width)
	assert.Equal(t, 12, form.FPS)
}

func TestFromQueryReadsAMalformedNumberAsUnset(t *testing.T) {
	t.Parallel()

	target := routes.PathMedia + "?" + values(
		routes.QueryAudioIndex, "second",
		routes.QueryWidth, "wide",
		routes.QueryFPS, "-",
	)

	form := fromQueryOf(t, target, false, false)

	assert.Zero(t, form.AudioIndex, "a field the server cannot read carries no value")
	assert.Zero(t, form.Width)
	assert.Zero(t, form.FPS)
}

func TestFromQueryUsesTheConfiguredToggles(t *testing.T) {
	t.Parallel()

	both := fromQueryOf(t, routes.PathMedia, true, true)
	assert.True(t, both.CropBlackBars)
	assert.True(t, both.PreserveHDR)

	neither := fromQueryOf(t, routes.PathMedia, false, false)
	assert.False(t, neither.CropBlackBars)
	assert.False(t, neither.PreserveHDR)
}

func TestFromQueryLetsTheQueryOverrideEachToggle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		giveCrop     string
		givePreserve string
		wantCrop     bool
		wantPreserve bool
	}{
		{
			name:         "a cleared crop arrives over a configured default of on",
			giveCrop:     routes.FormUnchecked,
			givePreserve: routes.FormChecked,
			wantCrop:     false,
			wantPreserve: true,
		},
		{
			name:         "a checked crop arrives over a configured default of off",
			giveCrop:     routes.FormChecked,
			givePreserve: routes.FormUnchecked,
			wantCrop:     true,
			wantPreserve: false,
		},
		{
			name:         "an unrecognized value reads as cleared",
			giveCrop:     "yes",
			givePreserve: "on",
			wantCrop:     false,
			wantPreserve: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			target := routes.PathMedia + "?" + values(
				routes.QueryCropBlackBars, test.giveCrop,
				routes.QueryPreserveHDR, test.givePreserve,
			)

			form := fromQueryOf(t, target, true, true)

			assert.Equal(t, test.wantCrop, form.CropBlackBars)
			assert.Equal(t, test.wantPreserve, form.PreserveHDR)
		})
	}
}

func TestFromQueryLeavesTheTogglesAloneWhenTheQueryIsSilent(t *testing.T) {
	t.Parallel()

	form := fromQueryOf(t, routes.PathMedia, true, true)

	assert.True(t, form.CropBlackBars,
		"an absent query parameter is not a submission, so the default stands")
	assert.True(t, form.PreserveHDR)
}

// TestMediaItemFormDefaultsKeepHDRFromTheProfile covers the Keep HDR box on a
// fresh form: it starts from the selected profile, or the default profile,
// and a carried box overrides it.
func TestMediaItemFormDefaultsKeepHDRFromTheProfile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		query string
		want  bool
	}{
		{name: "the default profile converts", query: "", want: false},
		{name: "High keeps HDR", query: values(routes.QueryQuality, "high"), want: true},
		{name: "Low converts", query: values(routes.QueryQuality, "low"), want: false},
		{
			name:  "a carried box wins",
			query: values(routes.QueryQuality, "low", routes.QueryPreserveHDR, routes.FormChecked),
			want:  true,
		},
		{
			name: "a carried empty box wins",
			query: values(
				routes.QueryQuality,
				"high",
				routes.QueryPreserveHDR,
				routes.FormUnchecked,
			),
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			form := mediaItemFormOf(t, routes.PathMedia+"?"+test.query, &config.Config{})

			assert.Equal(t, test.want, form.PreserveHDR)
		})
	}
}

func TestMediaItemFormKeepsTheCarriedNameOverTheConfiguredTitle(t *testing.T) {
	t.Parallel()

	target := routes.PathMedia + "?" + values(
		routes.QueryExportName,
		"",
		routes.QueryQuality,
		"high",
	)

	form := mediaItemFormOf(t, target, &config.Config{CropBlackBars: true})

	assert.Empty(t, form.Name, "the carried empty name survives the defaults")
	assert.True(t, form.CropBlackBars)
	assert.True(t, form.PreserveHDR)
}

func TestClipWindowReadsBothMarks(t *testing.T) {
	t.Parallel()

	window := clipWindowOf(t, routes.PathMedia+"?"+values(
		routes.QueryStart, "12.5", routes.QueryEnd, "42.25",
	))

	assert.Equal(t, 12500*time.Millisecond, window.Start)
	assert.Equal(t, 42250*time.Millisecond, window.End)
}

func TestClipWindowOffersASegmentWhenAMarkCannotBeRead(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		give      string
		wantStart time.Duration
		wantEnd   time.Duration
	}{
		{
			name:      "an unparseable start falls back to the head of the source",
			give:      values(routes.QueryStart, "start", routes.QueryEnd, "42.25"),
			wantStart: 0,
			wantEnd:   42250 * time.Millisecond,
		},
		{
			name:      "an unparseable end falls back to a default segment",
			give:      values(routes.QueryStart, "12.5", routes.QueryEnd, "end"),
			wantStart: 12500 * time.Millisecond,
			wantEnd:   22500 * time.Millisecond,
		},
		{
			name:      "an absent end falls back to a default segment",
			give:      values(routes.QueryStart, "12.5"),
			wantStart: 12500 * time.Millisecond,
			wantEnd:   22500 * time.Millisecond,
		},
		{
			name:      "an end at the start falls back to a default segment",
			give:      values(routes.QueryStart, "12.5", routes.QueryEnd, "12.5"),
			wantStart: 12500 * time.Millisecond,
			wantEnd:   22500 * time.Millisecond,
		},
		{
			name:      "an end before the start falls back to a default segment",
			give:      values(routes.QueryStart, "42.25", routes.QueryEnd, "12.5"),
			wantStart: 42250 * time.Millisecond,
			wantEnd:   52250 * time.Millisecond,
		},
		{
			name:      "no marks at all offer a segment from the head",
			give:      "",
			wantStart: 0,
			wantEnd:   10 * time.Second,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			window := clipWindowOf(t, routes.PathMedia+"?"+test.give)

			assert.Equal(t, test.wantStart, window.Start)
			assert.Equal(t, test.wantEnd, window.End,
				"the end mark is always ahead of the start, so the form can be submitted")
		})
	}
}

func TestNormalizeTypeAcceptsEveryOfferedExportType(t *testing.T) {
	t.Parallel()

	for _, kind := range []clip.Type{clip.TypeClip, clip.TypeGIF, clip.TypeScreenshot} {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, kind, NormalizeType(string(kind)))
		})
	}
}

func TestNormalizeTypeFallsBackToAClip(t *testing.T) {
	t.Parallel()

	tests := []string{"", "hologram", "CLIP", " video "}

	for _, give := range tests {
		t.Run(give, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, clip.TypeClip, NormalizeType(give),
				"a value the form does not offer cannot be selected by carrying it")
		})
	}
}

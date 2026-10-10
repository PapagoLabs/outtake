// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package profile

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fiber "github.com/gofiber/fiber/v3"

	"github.com/PapagoLabs/outtake/internal/clip"
	clipprofile "github.com/PapagoLabs/outtake/internal/clip/profile"
	"github.com/PapagoLabs/outtake/internal/store/database"
	"github.com/PapagoLabs/outtake/internal/web/routes"
)

type profileAnswer struct {
	status   int
	location string
	body     string
}

// profileTestService builds a clip profile service over a throwaway database.
//
// Parameters:
//   - t: The test the service belongs to.
//
// Returns:
//   - service: The service under test.
func profileTestService(t *testing.T) *clipprofile.Service {
	t.Helper()

	service, _ := profileTestServiceWithDB(t)

	return service
}

// profileTestServiceWithDB builds a clip profile service over a throwaway
// database the test may close.
//
// Parameters:
//   - t: The test the service belongs to.
//
// Returns:
//   - service: The service under test.
//   - db: The database it persists through.
func profileTestServiceWithDB(t *testing.T) (*clipprofile.Service, *database.DB) {
	t.Helper()

	db, err := database.New(t.TempDir() + "/clipprofile.db")
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	return clipprofile.New(db), db
}

// profileForm is a valid profile form body.
func profileForm(name string, makeDefault bool) string {
	form := url.Values{
		"name":      {name},
		"crf":       {"18"},
		"preset":    {"slow"},
		"audioKbps": {"320"},
		"maxWidth":  {"3840"},
	}
	if makeDefault {
		form.Set("isDefault", routes.FormChecked)
	}

	return form.Encode()
}

// storeProfile saves one profile directly, bypassing the form.
//
// Parameters:
//   - t: The test the profile belongs to.
//   - service: The service to save through.
//   - name: Name to store under.
//   - makeDefault: Whether the profile becomes the default.
//
// Returns:
//   - id: The identifier the service minted for the profile.
func storeProfile(
	t *testing.T,
	service *clipprofile.Service,
	name string,
	makeDefault bool,
) string {
	t.Helper()

	profile, err := service.Create(t.Context(), clipprofile.ProfileFields{
		Name:      name,
		CRF:       "18",
		Preset:    "slow",
		AudioKbps: "320",
		MaxWidth:  "3840",
		IsDefault: makeDefault,
	})
	require.NoError(t, err)

	return profile.ID
}

// profileNamed finds one stored profile by name.
//
// Parameters:
//   - t: The test the lookup belongs to.
//   - service: The service to read through.
//   - name: Name of the profile to find.
//
// Returns:
//   - profile: The stored profile.
func profileNamed(t *testing.T, service *clipprofile.Service, name string) clipprofile.Profile {
	t.Helper()

	for _, profile := range service.List(t.Context()) {
		if profile.Name == name {
			return profile
		}
	}

	require.FailNow(t, "no stored profile is named "+name)

	return clipprofile.Profile{}
}

// namedProfiles lists the profiles a test stored, which the seeded built-ins
// would otherwise be counted among.
//
// Parameters:
//   - t: The test the lookup belongs to.
//   - service: The service to read through.
//
// Returns:
//   - found: The profiles whose name is not one of the stored built-ins.
func namedProfiles(t *testing.T, service *clipprofile.Service) []clipprofile.Profile {
	t.Helper()

	builtIn := map[string]bool{"720p": true, "1080p": true, "4K": true, "4K HDR": true}

	found := make([]clipprofile.Profile, 0)

	for _, profile := range service.List(t.Context()) {
		if !builtIn[profile.Name] {
			found = append(found, profile)
		}
	}

	return found
}

// idOfNamedProfile finds the identifier of a stored profile by name.
//
// Parameters:
//   - t: The test the lookup belongs to.
//   - service: The service to read through.
//   - name: Name of the profile to find.
//
// Returns:
//   - id: The identifier the profile is stored under, empty when none is.
func idOfNamedProfile(t *testing.T, service *clipprofile.Service, name string) string {
	t.Helper()

	for _, profile := range service.List(t.Context()) {
		if profile.Name == name {
			return profile.ID
		}
	}

	return ""
}

// profileAnswer is what one served profile request produced.

// profileApp mounts the profiles routes on a fresh app.
//
// Parameters:
//   - handler: The handler under test.
//
// Returns:
//   - app: The app the routes are mounted on.
func profileApp(handler *Handler) *fiber.App {
	app := fiber.New()
	app.Get(routes.PathSettingsProfiles, handler.ClipProfiles)
	app.Post(routes.PathSettingsProfiles, handler.CreateClipProfile)
	app.Post(routes.PathSettingsProfiles+"/:id", handler.UpdateClipProfile)
	app.Post(routes.PathSettingsProfiles+"/:id/delete", handler.DeleteClipProfile)
	app.Post(routes.PathSettingsProfiles+"/:id/default", handler.SetDefaultClipProfile)

	return app
}

// getProfiles serves one profiles page request.
//
// Parameters:
//   - t: The test the request belongs to.
//   - handler: The handler under test.
//   - target: Request target, including any query string.
//
// Returns:
//   - answer: The status, location, and body the handler wrote.
func getProfiles(t *testing.T, handler *Handler, target string) profileAnswer {
	t.Helper()

	return send(t, profileApp(handler), http.MethodGet, target, "")
}

// postProfile serves one profile edit request.
//
// Parameters:
//   - t: The test the request belongs to.
//   - handler: The handler under test.
//   - path: Route to request.
//   - form: Form body, empty for no body.
//
// Returns:
//   - answer: The status, location, and body the handler wrote.
func postProfile(t *testing.T, handler *Handler, path, form string) profileAnswer {
	t.Helper()

	return send(t, profileApp(handler), http.MethodPost, path, form)
}

// send issues one request against a mounted app.
//
// Parameters:
//   - t: The test the request belongs to.
//   - app: The app to serve.
//   - method: HTTP method to issue.
//   - target: Request target, including any query string.
//   - form: Form body, empty for none.
//
// Returns:
//   - answer: The status, location, and body the response carried.
func send(t *testing.T, app *fiber.App, method, target, form string) profileAnswer {
	t.Helper()

	var req *http.Request

	if form == "" {
		req = httptest.NewRequestWithContext(t.Context(), method, target, nil)
	} else {
		req = httptest.NewRequestWithContext(
			t.Context(), method, target, strings.NewReader(form),
		)
		req.Header.Set(fiber.HeaderContentType, "application/x-www-form-urlencoded")
	}

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer closeBody(t, resp)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return profileAnswer{
		status:   resp.StatusCode,
		location: resp.Header.Get(fiber.HeaderLocation),
		body:     string(body),
	}
}

// flashOf reads the flash text a redirect carries back to the page.
//
// Parameters:
//   - t: The test the redirect belongs to.
//   - location: The redirect target.
//
// Returns:
//   - flash: The flash text, empty when none was carried.
func flashOf(t *testing.T, location string) string {
	t.Helper()

	parsed, err := url.Parse(location)
	require.NoError(t, err)

	return parsed.Query().Get(routes.QueryError)
}

// pathOf reads the path a redirect target names, without its flash query.
//
// Parameters:
//   - t: The test the redirect belongs to.
//   - location: The redirect target.
//
// Returns:
//   - path: The path the redirect lands on.
func pathOf(t *testing.T, location string) string {
	t.Helper()

	parsed, err := url.Parse(location)
	require.NoError(t, err)

	return parsed.Path
}

func TestNewKeepsTheServiceItWasGiven(t *testing.T) {
	t.Parallel()

	service := profileTestService(t)

	assert.Equal(t, &Handler{profiles: service}, New(service))
}

func TestClipProfilesRendersTheStoredProfiles(t *testing.T) {
	t.Parallel()

	service := profileTestService(t)
	storeProfile(t, service, "Archive", true)

	answer := getProfiles(t, New(service), routes.PathSettingsProfiles)

	require.Equal(t, fiber.StatusOK, answer.status)
	assert.Contains(t, answer.body, "Archive", "the stored profile is on the page")
	assert.Contains(t, answer.body, "Default", "the default profile says so")
	assert.Contains(t, answer.body, `name="preset"`, "the form offers the encoder presets")
	assert.Contains(t, answer.body, "4K", "the form offers every export resolution")
	assert.Contains(t, answer.body, `name="keepHdr"`, "the form offers the HDR setting")
	assert.Contains(t, answer.body, "Enable to maintain HDR quality from supported sources",
		"the HDR setting says what it does")
}

func TestClipProfilesRendersTheBuiltInProfiles(t *testing.T) {
	t.Parallel()

	answer := getProfiles(t, New(profileTestService(t)), routes.PathSettingsProfiles)

	require.Equal(t, fiber.StatusOK, answer.status)
	assert.Contains(t, answer.body, "Clip Profiles",
		"the page is still the settings page, not an error")
	assert.Contains(t, answer.body, `value="20"`,
		"the 1080p built-in is the stored default")
	assert.Contains(t, answer.body, "4K HDR", "the built-ins are named after what they produce")

	seeded := New(profileTestService(t))
	assert.Empty(t, namedProfiles(t, seeded.profiles),
		"nothing the user named has been stored yet")
}

func TestClipProfilesShowsTheCarriedFailure(t *testing.T) {
	t.Parallel()

	target := routes.PathSettingsProfiles + "?" + url.Values{
		routes.QueryError: {"create clip profile: validate profile: name is required"},
	}.Encode()

	answer := getProfiles(t, New(profileTestService(t)), target)

	require.Equal(t, fiber.StatusOK, answer.status)
	assert.Contains(t, answer.body, "name is required",
		"the reason the edit failed is what the user has to read")
}

func TestCreateClipProfileStoresThePostedProfile(t *testing.T) {
	t.Parallel()

	service := profileTestService(t)

	answer := postProfile(t, New(service), routes.PathSettingsProfiles,
		profileForm("Archive", true))

	require.Equal(t, fiber.StatusSeeOther, answer.status)
	assert.Equal(t, routes.PathSettingsProfiles, answer.location,
		"a clean edit lands back on the settings page with no flash")
	assert.Empty(t, flashOf(t, answer.location))

	stored := profileNamed(t, service, "Archive")
	assert.Equal(t, 18, stored.CRF)
	assert.Equal(t, "slow", stored.Preset)
	assert.Equal(t, 320, stored.AudioKbps)
	assert.Equal(t, 3840, stored.MaxWidth)
	assert.True(t, stored.IsDefault,
		"asking for the default moves it off the built-in that held it")
}

func TestCreateClipProfileReportsAnInvalidPost(t *testing.T) {
	t.Parallel()

	service := profileTestService(t)

	answer := postProfile(
		t, New(service), routes.PathSettingsProfiles,
		url.Values{
			"name":      {""},
			"crf":       {"18"},
			"preset":    {"slow"},
			"audioKbps": {"320"},
			"maxWidth":  {"3840"},
		}.Encode(),
	)

	require.Equal(t, fiber.StatusSeeOther, answer.status)
	assert.Equal(t, routes.PathSettingsProfiles, pathOf(t, answer.location))
	assert.Equal(
		t,
		"create clip profile: validate profile: name is required",
		flashOf(t, answer.location),
		"the failure the service reported reaches the user",
	)
	assert.Empty(t, namedProfiles(t, service), "nothing was stored")
}

func TestCreateClipProfileReportsAnUnusableEncoderSetting(t *testing.T) {
	t.Parallel()

	service := profileTestService(t)

	answer := postProfile(t, New(service), routes.PathSettingsProfiles,
		url.Values{
			"name":      {"Archive"},
			"crf":       {"99"},
			"preset":    {"slow"},
			"audioKbps": {"320"},
			"maxWidth":  {"3840"},
		}.Encode())

	require.Equal(t, fiber.StatusSeeOther, answer.status)
	assert.Contains(t, flashOf(t, answer.location), "crf must be between",
		"the message names the field the user has to correct")
	assert.Empty(t, namedProfiles(t, service))
}

func TestUpdateClipProfileSavesOverAnExistingProfile(t *testing.T) {
	t.Parallel()

	service := profileTestService(t)
	id := storeProfile(t, service, "Archive", false)

	answer := postProfile(t, New(service),
		routes.PathSettingsProfiles+"/"+id,
		url.Values{
			"name":      {"Draft"},
			"crf":       {"20"},
			"preset":    {"fast"},
			"audioKbps": {"192"},
			"maxWidth":  {"1920"},
			"keepHdr":   {"1"},
		}.Encode(),
	)

	require.Equal(t, fiber.StatusSeeOther, answer.status)
	assert.Empty(t, flashOf(t, answer.location))
	assert.Empty(t, idOfNamedProfile(t, service, "Archive"),
		"the edit renamed the profile rather than adding one")
	assert.Len(t, namedProfiles(t, service), 1)

	updated := profileNamed(t, service, "Draft")
	assert.Equal(t, id, updated.ID, "the edit kept the profile it was applied to")
	assert.Equal(t, 20, updated.CRF)
	assert.Equal(t, "fast", updated.Preset)
	assert.Equal(t, 192, updated.AudioKbps)
	assert.Equal(t, 1920, updated.MaxWidth)
	assert.True(t, updated.KeepHDR, "the edit saved the Keep HDR setting")
}

func TestUpdateClipProfileCarriesTheDefaultOverTheForm(t *testing.T) {
	t.Parallel()

	service := profileTestService(t)
	id := storeProfile(t, service, "Archive", true)
	require.True(t, profileNamed(t, service, "Archive").IsDefault)

	form := url.Values{
		"name":      {"Draft"},
		"crf":       {"18"},
		"preset":    {"slow"},
		"audioKbps": {"320"},
		"maxWidth":  {"3840"},
		"isDefault": {routes.FormUnchecked},
	}

	answer := postProfile(
		t, New(service), routes.PathSettingsProfiles+"/"+id, form.Encode(),
	)

	require.Equal(t, fiber.StatusSeeOther, answer.status)
	assert.Empty(t, flashOf(t, answer.location))

	assert.True(t, profileNamed(t, service, "Draft").IsDefault,
		"an edit restates the encode settings, not which profile is the default")
}

func TestUpdateClipProfileReportsAnUnknownProfile(t *testing.T) {
	t.Parallel()

	service := profileTestService(t)
	storeProfile(t, service, "Archive", false)

	answer := postProfile(t, New(service),
		routes.PathSettingsProfiles+"/no-such-profile",
		profileForm("Archive", false))

	require.Equal(t, fiber.StatusSeeOther, answer.status)
	assert.Equal(t, routes.PathSettingsProfiles, pathOf(t, answer.location))
	assert.Contains(t, flashOf(t, answer.location), "clip profile not found",
		"the reason the edit failed is what the user has to read")
}

func TestUpdateClipProfileReportsAnInvalidPost(t *testing.T) {
	t.Parallel()

	service := profileTestService(t)
	id := storeProfile(t, service, "Archive", false)

	answer := postProfile(t, New(service),
		routes.PathSettingsProfiles+"/"+id,
		url.Values{
			"name":      {"Archive"},
			"crf":       {"18"},
			"preset":    {"not-a-preset"},
			"audioKbps": {"320"},
			"maxWidth":  {"3840"},
		}.Encode(),
	)

	require.Equal(t, fiber.StatusSeeOther, answer.status)
	assert.Contains(t, flashOf(t, answer.location), "unknown encoder preset")

	unchanged := profileNamed(t, service, "Archive")
	assert.Equal(t, id, unchanged.ID, "the stored profile is untouched")
	assert.Equal(t, 18, unchanged.CRF)
}

func TestDeleteClipProfileRemovesAProfile(t *testing.T) {
	t.Parallel()

	service := profileTestService(t)
	storeProfile(t, service, "Archive", true)
	storeProfile(t, service, "Draft", false)

	doomed := idOfNamedProfile(t, service, "Draft")

	answer := postProfile(
		t, New(service), routes.PathSettingsProfiles+"/"+doomed+"/delete", "",
	)

	require.Equal(t, fiber.StatusSeeOther, answer.status)
	assert.Equal(t, routes.PathSettingsProfiles, pathOf(t, answer.location))
	assert.Empty(t, flashOf(t, answer.location))

	assert.Empty(t, idOfNamedProfile(t, service, "Draft"))
	assert.NotEmpty(t, idOfNamedProfile(t, service, "Archive"),
		"the other profile is still there")
	assert.Len(t, namedProfiles(t, service), 1)
}

func TestDeleteClipProfileReportsAStoreItCannotReach(t *testing.T) {
	t.Parallel()

	service, db := profileTestServiceWithDB(t)
	id := storeProfile(t, service, "Archive", true)

	require.NoError(t, db.Close())

	answer := postProfile(
		t, New(service), routes.PathSettingsProfiles+"/"+id+"/delete", "",
	)

	require.Equal(t, fiber.StatusSeeOther, answer.status)
	assert.Contains(t, flashOf(t, answer.location), "delete clip profile",
		"a delete that could not run is reported rather than reported as a success")
}

func TestDeleteClipProfileReportsAnUnknownProfile(t *testing.T) {
	t.Parallel()

	service := profileTestService(t)
	storeProfile(t, service, "Archive", true)
	storeProfile(t, service, "Draft", false)

	answer := postProfile(
		t, New(service), routes.PathSettingsProfiles+"/no-such-profile/delete", "",
	)

	require.Equal(t, fiber.StatusSeeOther, answer.status)
	assert.Contains(t, flashOf(t, answer.location), "clip profile not found")
}

func TestSetDefaultClipProfileMovesTheFlag(t *testing.T) {
	t.Parallel()

	service := profileTestService(t)
	storeProfile(t, service, "Archive", true)
	storeProfile(t, service, "Draft", false)

	promoted := idOfNamedProfile(t, service, "Draft")

	answer := postProfile(
		t, New(service), routes.PathSettingsProfiles+"/"+promoted+"/default", "",
	)

	require.Equal(t, fiber.StatusSeeOther, answer.status)
	assert.Empty(t, flashOf(t, answer.location))

	assert.True(t, profileNamed(t, service, "Draft").IsDefault)
	assert.False(t, profileNamed(t, service, "Archive").IsDefault,
		"only one profile is default at a time")

	defaults := service.List(t.Context())
	assert.Equal(t, promoted, defaults[0].ID,
		"the default sorts first, so a clip asks for it first")
}

func TestSetDefaultClipProfileReportsAnUnknownProfile(t *testing.T) {
	t.Parallel()

	service := profileTestService(t)
	storeProfile(t, service, "Archive", true)

	answer := postProfile(
		t, New(service), routes.PathSettingsProfiles+"/no-such-profile/default", "",
	)

	require.Equal(t, fiber.StatusSeeOther, answer.status)
	assert.Contains(t, flashOf(t, answer.location), "clip profile not found",
		"a default that was never set is reported rather than silently ignored")
}

func TestRedirectToProfilesCarriesTheFailureOnTheQuery(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Get(routes.PathSettingsProfiles, func(ctx fiber.Ctx) error {
		return redirectToProfiles(ctx, assert.AnError)
	})

	answer := send(t, app, http.MethodGet, routes.PathSettingsProfiles, "")

	require.Equal(t, fiber.StatusSeeOther, answer.status)
	assert.Equal(t, assert.AnError.Error(), flashOf(t, answer.location))
}

func TestRedirectToProfilesSendsACleanEditWithoutAQuery(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	app.Get(routes.PathSettingsProfiles, func(ctx fiber.Ctx) error {
		return redirectToProfiles(ctx, nil)
	})

	answer := send(t, app, http.MethodGet, routes.PathSettingsProfiles, "")

	require.Equal(t, fiber.StatusSeeOther, answer.status)
	assert.Equal(t, routes.PathSettingsProfiles, answer.location,
		"nothing to report means no flash is carried")
}

func TestStoredProfileKeepsItsCreationStampAcrossAnEdit(t *testing.T) {
	t.Parallel()

	service := profileTestService(t)
	id := storeProfile(t, service, "Archive", true)

	created := profileNamed(t, service, "Archive").CreatedAt
	require.False(t, created.IsZero())

	time.Sleep(time.Millisecond)

	_, err := service.Update(t.Context(), id, clipprofile.ProfileFields{
		Name:      "Draft",
		CRF:       "20",
		Preset:    "fast",
		AudioKbps: "192",
		MaxWidth:  "1920",
	})
	require.NoError(t, err)

	updated := profileNamed(t, service, "Draft").CreatedAt
	assert.Equal(t, created.UTC().Format(time.RFC3339),
		updated.UTC().Format(time.RFC3339),
		"an edit restates the encode settings, not when the profile was made")
}

func TestProfileWidthChoicesCoverEveryExportResolution(t *testing.T) {
	t.Parallel()

	for _, width := range clip.OutputWidths {
		assert.NotEmpty(t, clip.OutputWidthLabel(width))
	}

	answer := getProfiles(t, New(profileTestService(t)), routes.PathSettingsProfiles)

	require.Equal(t, fiber.StatusOK, answer.status)

	for _, width := range clip.OutputWidths {
		assert.Contains(t, answer.body, clip.OutputWidthLabel(width),
			"every supported resolution has to be offered or the user cannot choose it")
	}
}

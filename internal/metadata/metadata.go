// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package metadata

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"sync"
	"time"
)

// VersionInfo holds complete version data for display/export.
type VersionInfo struct {
	// Name is the application name.
	Name string `json:"name"`
	// Version is the semantic version string.
	Version string `json:"version"`
	// CommitSHA is the git commit hash at build time (omitted if empty).
	CommitSHA string `json:"commitSha,omitempty"`
	// BuildTime is the timestamp of the build (omitted if empty).
	BuildTime string `json:"buildTime,omitempty"`
	// GoVersion is the Go toolchain version used for the build.
	GoVersion string `json:"goVersion,omitempty"`
	// OS is the operating system the build was compiled for.
	OS string `json:"os"`
	// Arch is the CPU architecture the build was compiled for.
	Arch string `json:"arch"`
}

const (
	// Name is the application name.
	Name = "outtake"
	// DefaultVersion is used when no version information is available.
	DefaultVersion = "dev"
)

var (
	// Version is the full semantic version string.
	Version = DefaultVersion
	// CommitSHA is the git commit hash at build time.
	CommitSHA = ""
	// BuildTime is the timestamp of the build.
	BuildTime = ""

	// versionOnce ensures version is initialized only once.
	versionOnce sync.Once
)

// initVersion populates Version from Go build info if available.
func initVersion() {
	info, ok := debug.ReadBuildInfo()
	if ok && info.Main.Version != "(devel)" {
		Version = info.Main.Version
	}
}

// String returns a human-readable version string.
//
// Returns:
//   - string: The version string, optionally including the commit SHA.
func String() string {
	versionOnce.Do(initVersion)

	if CommitSHA != "" {
		return fmt.Sprintf("%s (%s)", Version, CommitSHA)
	}

	return Version
}

// GetGoVersion returns the Go toolchain version, or "unknown" if unavailable.
//
// Returns:
//   - string: The Go version string from build info, or "unknown".
func GetGoVersion() string {
	info, ok := debug.ReadBuildInfo()
	if ok && info.GoVersion != "" {
		return info.GoVersion
	}

	return "unknown"
}

// ConvertToLocal converts an RFC3339 UTC timestamp to a local time string.
//
// Parameters:
//   - utcStr: The UTC timestamp in RFC3339 format.
//
// Returns:
//   - string: The formatted local time string, or empty string if input is empty.
//     Returns the original string if parsing fails.
func ConvertToLocal(utcStr string) string {
	if utcStr == "" {
		return ""
	}

	utcTime, err := time.Parse(time.RFC3339, utcStr)
	if err != nil {
		return utcStr
	}

	return utcTime.Format("2006-01-02 15:04:05 UTC")
}

// GetInfo returns populated VersionInfo with raw version (no commit suffix).
//
// Returns:
//   - VersionInfo: Structured version data for display or serialization.
func GetInfo() VersionInfo {
	commitSHA := ""
	if CommitSHA != "" {
		commitSHA = CommitSHA
	}

	buildTime := ""
	if BuildTime != "" {
		buildTime = ConvertToLocal(BuildTime)
	}

	info := VersionInfo{
		Name:      Name,
		Version:   Version,
		CommitSHA: commitSHA,
		BuildTime: buildTime,
		GoVersion: GetGoVersion(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}

	return info
}

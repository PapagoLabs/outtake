package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReleaseURLs(t *testing.T) {
	t.Parallel()

	archiveURL, checksumURL, filename := releaseURLs("amd64")
	if filename != "ffmpeg-release-amd64-static.tar.xz" {
		t.Fatalf("filename=%q", filename)
	}
	if archiveURL != releaseBase+filename {
		t.Fatalf("archiveURL=%q", archiveURL)
	}
	if checksumURL != archiveURL+".md5" {
		t.Fatalf("checksumURL=%q", checksumURL)
	}

	_, _, arm := releaseURLs("arm64")
	if arm != "ffmpeg-release-arm64-static.tar.xz" {
		t.Fatalf("arm=%q", arm)
	}
}

func TestFindBin(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	loose := filepath.Join(root, "ffmpeg-7.0-amd64-static")
	if err := os.MkdirAll(loose, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(loose, "ffmpeg"), []byte("static"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := findBin(root, "ffmpeg")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(loose, "ffmpeg") {
		t.Fatalf("got %q", got)
	}
}

func TestParseChecksum(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "sum.md5")
	content := "abc123  ffmpeg-release-amd64-static.tar.xz\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := parseChecksum(path, "ffmpeg-release-amd64-static.tar.xz")
	if err != nil {
		t.Fatal(err)
	}
	if got != "abc123" {
		t.Fatalf("got %q", got)
	}
}

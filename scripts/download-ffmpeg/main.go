package main

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mholt/archives"
)

const (
	releaseBase = "https://johnvansickle.com/ffmpeg/releases/"
	defaultDest = "/build"
)

// main downloads static ffmpeg and ffprobe into FFMPEG_DEST (default /build).
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// run downloads, verifies, extracts, and installs ffmpeg and ffprobe.
//
// Returns:
//   - err: The error, if any.
func run() error {
	ctx := context.Background()

	archiveURL, checksumURL, filename := releaseURLs(targetArch())

	archive, err := download(archiveURL)
	if err != nil {
		return fmt.Errorf("download archive: %w", err)
	}
	defer os.Remove(archive)

	checksums, err := download(checksumURL)
	if err != nil {
		return fmt.Errorf("download checksums: %w", err)
	}
	defer os.Remove(checksums)

	expected, err := parseChecksum(checksums, filename)
	if err != nil {
		return err
	}

	actual, err := md5File(archive)
	if err != nil {
		return err
	}

	if actual != expected {
		return fmt.Errorf("checksum mismatch:\n  expected: %s\n  actual:   %s", expected, actual)
	}

	fmt.Println("checksum verified:", actual)

	extractDir, err := os.MkdirTemp("", "ffmpeg-extract-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(extractDir)

	if err := extractTarXz(ctx, archive, extractDir); err != nil {
		return fmt.Errorf("extract archive: %w", err)
	}

	destDir := os.Getenv("FFMPEG_DEST")
	if destDir == "" {
		destDir = defaultDest
	}

	for _, name := range []string{"ffmpeg", "ffprobe"} {
		src, findErr := findBin(extractDir, name)
		if findErr != nil {
			return fmt.Errorf("find %s: %w", name, findErr)
		}

		dst := filepath.Join(destDir, name)
		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("copy %s: %w", name, err)
		}

		fmt.Println("installed", dst)
	}

	return nil
}

// download fetches url into a temporary file.
//
// Parameters:
//   - url: Remote file URL.
//
// Returns:
//   - path: Local temporary file path.
//   - err: The error, if any.
func download(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: %s", url, resp.Status)
	}

	f, err := os.CreateTemp("", "ffmpeg-download-*")
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	f.Close()
	return f.Name(), nil
}

// targetArch returns the ffmpeg build architecture.
//
// Uses FFMPEG_ARCH when set; otherwise [runtime.GOARCH].
//
// Returns:
//   - arch: Target architecture name (amd64 or arm64).
func targetArch() string {
	if arch := os.Getenv("FFMPEG_ARCH"); arch != "" {
		return arch
	}
	return runtime.GOARCH
}

// releaseURLs builds johnvansickle release URLs for goarch.
//
// Parameters:
//   - goarch: Go architecture (amd64 or arm64).
//
// Returns:
//   - archiveURL: Archive download URL.
//   - checksumURL: MD5 checksum file URL.
//   - filename: Archive basename.
func releaseURLs(goarch string) (archiveURL, checksumURL, filename string) {
	arch := "amd64"
	if goarch == "arm64" {
		arch = "arm64"
	}

	filename = "ffmpeg-release-" + arch + "-static.tar.xz"
	archiveURL = releaseBase + filename
	checksumURL = archiveURL + ".md5"

	return archiveURL, checksumURL, filename
}

// parseChecksum reads the MD5 hex digest for filename from a checksum file.
//
// Parameters:
//   - path: Local checksum file path.
//   - filename: Archive basename to match.
//
// Returns:
//   - sum: Expected MD5 hex digest.
//   - err: The error, if any.
func parseChecksum(path, filename string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if filepath.Base(parts[len(parts)-1]) == filename || len(parts) == 1 {
			return parts[0], nil
		}
	}
	return "", fmt.Errorf("checksum for %s not found", filename)
}

// md5File returns the MD5 hex digest of a file.
//
// Parameters:
//   - path: File to hash.
//
// Returns:
//   - sum: MD5 hex digest.
//   - err: The error, if any.
func md5File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// extractTarXz extracts a .tar.xz archive into dest.
//
// Parameters:
//   - ctx: Cancellation context.
//   - archivePath: Path to the archive file.
//   - dest: Destination directory.
//
// Returns:
//   - err: The error, if any.
func extractTarXz(ctx context.Context, archivePath, dest string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	format, stream, err := archives.Identify(ctx, archivePath, f)
	if err != nil {
		return err
	}

	extractor, ok := format.(archives.Extractor)
	if !ok {
		return fmt.Errorf("format %T does not support extraction", format)
	}

	return extractor.Extract(ctx, stream, func(_ context.Context, info archives.FileInfo) error {
		name := path.Clean(info.NameInArchive)
		if strings.HasPrefix(name, "..") {
			return fmt.Errorf("invalid path: %s", name)
		}

		fullPath := filepath.Join(dest, name)

		if info.IsDir() {
			return os.MkdirAll(fullPath, info.Mode())
		}

		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			return err
		}

		src, err := info.Open()
		if err != nil {
			return err
		}
		defer src.Close()

		dst, err := os.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		defer dst.Close()

		_, err = io.Copy(dst, src)
		return err
	})
}

// findBin locates an executable named name under root.
//
// Prefers a bin/ directory when multiple matches exist.
//
// Parameters:
//   - root: Directory tree to search.
//   - name: Executable basename (ffmpeg or ffprobe).
//
// Returns:
//   - path: Absolute path to the binary.
//   - err: The error, if any.
func findBin(root, name string) (string, error) {
	var found string

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() || entry.Name() != name {
			return nil
		}

		if found == "" || filepath.Base(filepath.Dir(path)) == "bin" {
			found = path
		}

		if filepath.Base(filepath.Dir(path)) == "bin" {
			return filepath.SkipAll
		}

		return nil
	})
	if err != nil {
		return "", err
	}

	if found == "" {
		return "", fmt.Errorf("%s not found under %s", name, root)
	}

	return found, nil
}

// copyFile copies src to dst with executable permissions.
//
// Parameters:
//   - src: Source file path.
//   - dst: Destination file path.
//
// Returns:
//   - err: The error, if any.
func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o755)
}

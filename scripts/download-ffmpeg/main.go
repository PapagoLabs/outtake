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
	extractDir  = "/tmp/ffmpeg-extract"
	defaultDest = "/build"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	archiveURL, checksumURL, filename := releaseURLs(runtime.GOARCH)

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

	if err := os.RemoveAll(extractDir); err != nil {
		return err
	}
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return err
	}

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

	os.RemoveAll(extractDir)
	return nil
}

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

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o755)
}

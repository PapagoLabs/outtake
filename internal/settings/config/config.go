// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package config is the settings subdomain that loads the application
// configuration, from the config file, the environment, and the defaults it
// seeds when neither carries a value.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/adrg/xdg"
	"github.com/spf13/viper"

	"github.com/PapagoLabs/outtake/internal/clip"
	"github.com/PapagoLabs/outtake/internal/timecode"
)

// Config represents the application configuration.
type Config struct {
	// ListenAddr is the address the server listens on.
	ListenAddr string `mapstructure:"listen-addr"`
	// DatabasePath is the path to the database file.
	DatabasePath string `mapstructure:"database-path"`
	// DatabaseBackend selects sqlite (default) or postgres/pgx.
	DatabaseBackend string `mapstructure:"database-backend"`
	// DatabaseURL is the Postgres DSN when DatabaseBackend is postgres.
	DatabaseURL string `mapstructure:"database-url"`
	// StoragePath is the path to the storage directory.
	StoragePath string `mapstructure:"storage-path"`
	// StorageBackend selects filesystem (default) or s3.
	StorageBackend string `mapstructure:"storage-backend"`
	// S3Endpoint is the S3-compatible API endpoint.
	S3Endpoint string `mapstructure:"s3-endpoint"`
	// S3Bucket is the target bucket.
	S3Bucket string `mapstructure:"s3-bucket"`
	// S3Region is the S3 region.
	S3Region string `mapstructure:"s3-region"`
	// S3AccessKey is the static access key.
	S3AccessKey string `mapstructure:"s3-access-key"`
	// S3SecretKey is the static secret key.
	S3SecretKey string `mapstructure:"s3-secret-key"`
	// S3UsePathStyle forces path-style URLs for S3-compatible stores.
	S3UsePathStyle bool `mapstructure:"s3-use-path-style"`
	// FFmpegPath is the path to FFmpeg.
	FFmpegPath string `mapstructure:"ffmpeg-path"`
	// FFprobePath is the path to FFprobe.
	FFprobePath string `mapstructure:"ffprobe-path"`
	// FFmpegTimeout is how long one ffmpeg run may take, zero to scale the
	// limit with the clip's length.
	//nolint:tagliatelle // The key ends in -sec like session-poll-sec, since its value is a second count.
	FFmpegTimeout time.Duration `mapstructure:"ffmpeg-timeout-sec"`
	// LogLevel is the log level.
	LogLevel string `mapstructure:"log-level"`
	// Env is the environment.
	Env string `mapstructure:"env"`
	// SessionPoll is the session poll interval.
	//nolint:tagliatelle // The key stays session-poll-sec, so existing environments keep binding.
	SessionPoll time.Duration `mapstructure:"session-poll-sec"`
	// NumWorkers is the number of workers.
	NumWorkers int `mapstructure:"num-workers"`
	// MaxConcurrentPreviews bounds simultaneous preview encodes.
	MaxConcurrentPreviews int `mapstructure:"max-concurrent-previews"`
	// MaxClipDur is the longest clip the interface will offer.
	MaxClipDur time.Duration `mapstructure:"max-clip-dur"`
	// CropBlackBars is the default for trimming letterbox/pillarbox bars.
	CropBlackBars bool `mapstructure:"crop-black-bars"`
	// WebSafeColor is the default for HDR tone-mapping on saved clips.
	WebSafeColor bool `mapstructure:"web-safe-color"`
	// PreserveHDR keeps an HDR source as it is instead of tone mapping it to Rec.709.
	PreserveHDR bool `mapstructure:"preserve-hdr"`
	// PlexServerURL is the Plex server URL.
	PlexServerURL string `mapstructure:"plex-server-url"`
	// PlexToken is the Plex token.
	PlexToken string `mapstructure:"plex-token"`
	// PlexClientID is the Plex client ID.
	PlexClientID string `mapstructure:"plex-client-id"`
	// PublicBaseURL is the externally reachable base URL for Plex callbacks.
	PublicBaseURL string `mapstructure:"public-base-url"`
	// AllowedHosts lists further host names the server answers to, separated by
	// commas or spaces. A leading dot covers a whole domain and "*" turns the
	// host check off.
	AllowedHosts string `mapstructure:"allowed-hosts"`
	// PlexMediaRoot is the media path prefix as reported by Plex.
	PlexMediaRoot string `mapstructure:"plex-media-root"`
	// LocalMediaRoot is the local path prefix that replaces PlexMediaRoot.
	LocalMediaRoot string `mapstructure:"local-media-root"`
}

// EnvE2E is the environment the end-to-end suite runs the app in. It skips
// sign-in and reads media ids as local file paths, so the app only accepts it
// on a loopback listen address.
const EnvE2E = "e2e"

const (
	// appName is the application name.
	appName = "outtake"
	// defaultListenAddr is the default listen address.
	defaultListenAddr = "0.0.0.0:8080"
	// defaultLogLevel is the default log level.
	defaultLogLevel = "info"
	// defaultEnv is the default environment.
	defaultEnv = "production"
	// defaultFFmpegPath is the default FFmpeg path.
	defaultFFmpegPath = "ffmpeg"
	// defaultFFprobePath is the default FFprobe path.
	defaultFFprobePath = "ffprobe"
	// defaultSessionPoll is the default session poll interval.
	defaultSessionPoll = 10 * time.Second
	// defaultNumWorkers is the default number of workers.
	defaultNumWorkers = 2
	// defaultMaxConcurrentPreviews is the default number of simultaneous previews.
	defaultMaxConcurrentPreviews = 2
	// defaultDirPerms is the default directory permissions.
	defaultDirPerms = 0o755
	// defaultDatabaseBackend is the default database backend.
	defaultDatabaseBackend = "sqlite"
	// defaultStorageBackend is the default storage backend.
	defaultStorageBackend = "filesystem"
	// defaultS3Region is the default S3 region.
	defaultS3Region = "us-east-1"
	// secondsBitSize is the bit size used when parsing a second count.
	secondsBitSize = 64
)

const (
	// slash separates the parts of a Windows path once its backslashes are
	// replaced.
	slash = "/"
)

var (
	// windowsVolumePattern matches the start of a Windows path: a drive
	// letter, or a UNC share as Windows writes it, with backslashes. A Linux
	// path may start with two slashes, so a slashed share is not matched.
	windowsVolumePattern = regexp.MustCompile(`^(?:[A-Za-z]:[\\/]|\\\\[^\\/]+\\[^\\/]+)`)

	// slashedVolumePattern matches the volume of a Windows path that has had
	// its backslashes turned into slashes.
	slashedVolumePattern = regexp.MustCompile(`^(?:[A-Za-z]:|//[^/]+/[^/]+)`)
)

// ConfigPath returns the configuration file path.
//
// Returns:
//   - path: Path of the user's configuration file.
func ConfigPath() string {
	return filepath.Join(xdg.ConfigHome, appName, "config.yaml")
}

// DatabasePath returns the database file path.
//
// Returns:
//   - path: Default database file under the user's data directory.
func DatabasePath() string {
	return filepath.Join(xdg.DataHome, appName, "outtake.db")
}

// StoragePath returns the storage directory path.
//
// Returns:
//   - path: Default output directory under the user's data directory.
func StoragePath() string {
	return filepath.Join(xdg.DataHome, appName, "output")
}

// Load loads the configuration.
//
// Parameters:
//   - configFile: Explicit config file path, or empty to rely on defaults and
//     environment.
//
// Returns:
//   - cfg: The loaded configuration, with its directories created.
//   - err: Wrapped error when the file, the environment, or a directory fails.
func Load(configFile string) (*Config, error) {
	viperInstance := viper.New()

	viperInstance.SetEnvPrefix("OUTTAKE")
	viperInstance.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viperInstance.AutomaticEnv()

	if configFile != "" && fileExists(configFile) {
		viperInstance.SetConfigFile(configFile)

		err := viperInstance.ReadInConfig()
		if err != nil {
			return nil, fmt.Errorf("read config: %w", err)
		}
	}

	setDefaults(viperInstance)

	err := bindEnv(viperInstance)
	if err != nil {
		return nil, fmt.Errorf("bind env: %w", err)
	}

	cfg := emptyConfig()

	err = viperInstance.Unmarshal(cfg, viper.DecodeHook(secondsToDurationHook))
	if err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	// At least one render worker runs, or every clip would wait forever.
	cfg.NumWorkers = max(cfg.NumWorkers, 1)

	err = ensureDirs(cfg)
	if err != nil {
		return nil, fmt.Errorf("ensure dirs: %w", err)
	}

	return cfg, nil
}

// emptyConfig returns a zero Config including S3 and database keys.
//
// Returns:
//   - cfg: A Config with every field at its zero value.
func emptyConfig() *Config {
	return &Config{
		ListenAddr:      "",
		DatabasePath:    "",
		DatabaseBackend: "",
		DatabaseURL:     "",
		StoragePath:     "",
		StorageBackend:  "",
		S3Endpoint:      "",
		S3Bucket:        "",
		S3Region:        "",
		S3AccessKey:     "",
		S3SecretKey:     "",
		S3UsePathStyle:  false,
		FFmpegPath:      "",
		FFprobePath:     "",
		FFmpegTimeout:   0,
		LogLevel:        "",
		Env:             "",
		SessionPoll:     0,
		NumWorkers:      0,
		MaxClipDur:      0,
		CropBlackBars:   false,
		WebSafeColor:    false,
		PreserveHDR:     false,
		PlexServerURL:   "",
		PlexToken:       "",
		PlexClientID:    "",
		PublicBaseURL:   "",
		AllowedHosts:    "",
		PlexMediaRoot:   "",
		LocalMediaRoot:  "",
	}
}

// setDefaults sets the default configuration values.
//
// Parameters:
//   - viperInstance: Viper instance to seed.
func setDefaults(viperInstance *viper.Viper) {
	viperInstance.SetDefault("listen-addr", defaultListenAddr)
	viperInstance.SetDefault("database-path", DatabasePath())
	viperInstance.SetDefault("database-backend", defaultDatabaseBackend)
	viperInstance.SetDefault("database-url", "")
	viperInstance.SetDefault("storage-path", StoragePath())
	viperInstance.SetDefault("storage-backend", defaultStorageBackend)
	viperInstance.SetDefault("s3-endpoint", "")
	viperInstance.SetDefault("s3-bucket", "")
	viperInstance.SetDefault("s3-region", defaultS3Region)
	viperInstance.SetDefault("s3-access-key", "")
	viperInstance.SetDefault("s3-secret-key", "")
	viperInstance.SetDefault("s3-use-path-style", true)
	viperInstance.SetDefault("ffmpeg-path", defaultFFmpegPath)
	viperInstance.SetDefault("ffprobe-path", defaultFFprobePath)
	viperInstance.SetDefault("ffmpeg-timeout-sec", 0)
	viperInstance.SetDefault("log-level", defaultLogLevel)
	viperInstance.SetDefault("env", defaultEnv)
	viperInstance.SetDefault("session-poll-sec", int(defaultSessionPoll.Seconds()))
	viperInstance.SetDefault("num-workers", defaultNumWorkers)
	viperInstance.SetDefault("max-concurrent-previews", defaultMaxConcurrentPreviews)
	viperInstance.SetDefault("max-clip-dur", int(clip.MaxDuration.Seconds()))
	viperInstance.SetDefault("crop-black-bars", false)
	viperInstance.SetDefault("preserve-hdr", false)
	viperInstance.SetDefault("web-safe-color", false)
	viperInstance.SetDefault("allowed-hosts", "")
	viperInstance.SetDefault("plex-media-root", "")
	viperInstance.SetDefault("local-media-root", "")
}

// secondsToDurationHook reads a bare integer config value as a count of seconds.
//
// Parameters:
//   - from: Source value type.
//   - to: Target value type.
//   - data: Decoded value.
//
// Returns:
//   - value: The value to decode into the target, scaled when it is an integer.
//   - err: Always nil, so the hook never blocks a decode.
func secondsToDurationHook(from, to reflect.Type, data any) (any, error) {
	if to != reflect.TypeFor[time.Duration]() {
		return data, nil
	}

	seconds, ok := durationSeconds(from, data)
	if !ok {
		return data, nil
	}

	return timecode.FromSeconds(seconds).Duration(), nil
}

// durationSeconds reads a config value that is documented as a count of seconds.
//
// Environment values arrive as strings. Leaving those for mapstructure to coerce
// into a [time.Duration] counts them as nanoseconds.
//
// Parameters:
//   - from: Source value type.
//   - data: Decoded value.
//
// Returns:
//   - seconds: The value in seconds.
//   - ok: False when the value is not a bare second count.
func durationSeconds(from reflect.Type, data any) (float64, bool) {
	switch from.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(reflect.ValueOf(data).Int()), true
	case reflect.Float32, reflect.Float64:
		return reflect.ValueOf(data).Float(), true
	case reflect.String:
		text, ok := data.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return 0, false
		}

		seconds, err := strconv.ParseFloat(strings.TrimSpace(text), secondsBitSize)
		if err != nil {
			return 0, false
		}

		return seconds, true
	default:
		return 0, false
	}
}

// bindEnv registers OUTTAKE_* environment keys so Unmarshal can see them.
//
// Parameters:
//   - viperInstance: Viper instance to bind keys on.
//
// Returns:
//   - err: Wrapped error naming the key that could not be bound.
func bindEnv(viperInstance *viper.Viper) error {
	keys := []string{
		"listen-addr",
		"database-path",
		"database-backend",
		"database-url",
		"storage-path",
		"storage-backend",
		"s3-endpoint",
		"s3-bucket",
		"s3-region",
		"s3-access-key",
		"s3-secret-key",
		"s3-use-path-style",
		"ffmpeg-path",
		"ffprobe-path",
		"ffmpeg-timeout-sec",
		"log-level",
		"env",
		"session-poll-sec",
		"num-workers",
		"max-concurrent-previews",
		"max-clip-dur",
		"crop-black-bars",
		"web-safe-color",
		"plex-server-url",
		"plex-token",
		"plex-client-id",
		"public-base-url",
		"allowed-hosts",
		"plex-media-root",
		"local-media-root",
	}

	for _, key := range keys {
		err := viperInstance.BindEnv(key)
		if err != nil {
			return fmt.Errorf("bind env %s: %w", key, err)
		}
	}

	return nil
}

// AllowedHostList splits AllowedHosts into the entries it lists.
//
// Returns:
//   - hosts: The listed host names, domains, and wildcard, in order.
func (cfg *Config) AllowedHostList() []string {
	return strings.FieldsFunc(cfg.AllowedHosts, func(char rune) bool {
		return char == ',' || unicode.IsSpace(char)
	})
}

// PublicURL returns the base URL used for Plex OAuth callbacks.
//
// Returns:
//   - url: The configured public base URL, or one derived from the listen
//     address.
func (cfg *Config) PublicURL() string {
	if cfg.PublicBaseURL != "" {
		return strings.TrimRight(cfg.PublicBaseURL, "/")
	}

	addr := cfg.ListenAddr
	switch {
	case strings.HasPrefix(addr, ":"):
		return "http://localhost" + addr
	case strings.HasPrefix(addr, "0.0.0.0:"):
		return "http://localhost:" + strings.TrimPrefix(addr, "0.0.0.0:")
	default:
		return "http://" + addr
	}
}

// RemapMediaPath maps a Plex filesystem path onto the local mount.
//
// Parameters:
//   - plexPath: Path as the Plex server reports it.
//
// Returns:
//   - local: The path to read on this host. Empty when the mapped path would
//     leave the local mount.
func (cfg *Config) RemapMediaPath(plexPath string) string {
	if cfg.LocalMediaRoot == "" {
		return plexPath
	}

	rel, ok := cfg.plexRelative(plexPath)
	if !ok {
		return plexPath
	}

	rel = strings.TrimPrefix(rel, string(filepath.Separator))

	local := filepath.Clean(cfg.LocalMediaRoot)
	if rel == "" {
		return local
	}

	mapped := filepath.Clean(filepath.Join(local, rel))
	if mapped != local && !strings.HasPrefix(mapped, local+string(filepath.Separator)) {
		// A .. segment would otherwise leave the local mount. An empty result
		// cannot be opened, which beats reading a file outside the mount.
		return ""
	}

	return mapped
}

// plexRelative returns a Plex path relative to the configured Plex root. A
// Windows path, one that starts with a drive letter or a UNC share, has its
// backslashes turned into slashes, is matched against the root without regard
// to case as Windows does, and without a root loses its volume, so it can sit
// under the local mount.
//
// Parameters:
//   - plexPath: Path as the Plex server reports it.
//
// Returns:
//   - rel: The path relative to the Plex root, which may still contain ..
//     segments.
//   - ok: False when a root is configured and the path is not under it.
func (cfg *Config) plexRelative(plexPath string) (string, bool) {
	path, windows := windowsPath(plexPath)
	if !windows {
		if cfg.PlexMediaRoot == "" {
			return plexPath, true
		}

		return trimMediaRoot(plexPath, cfg.PlexMediaRoot)
	}

	if cfg.PlexMediaRoot == "" {
		return strings.TrimPrefix(path, windowsVolume(path)), true
	}

	return trimWindowsRoot(path, strings.ReplaceAll(cfg.PlexMediaRoot, `\`, slash))
}

// windowsPath reports whether a path is a Windows path, one starting with a
// drive letter or a UNC share, and returns it with forward slashes.
//
// Parameters:
//   - path: Path as the Plex server reports it.
//
// Returns:
//   - slashed: The path with forward slashes, or path unchanged when it is
//     not a Windows path.
//   - windows: True for a drive letter or UNC path.
func windowsPath(path string) (string, bool) {
	if !windowsVolumePattern.MatchString(path) {
		return path, false
	}

	return strings.ReplaceAll(path, `\`, slash), true
}

// windowsVolume returns the drive letter or UNC share a slashed Windows path
// starts with.
//
// Parameters:
//   - path: A Windows path with forward slashes.
//
// Returns:
//   - volume: Such as "D:" or "//nas/share", empty when there is none.
func windowsVolume(path string) string {
	return slashedVolumePattern.FindString(path)
}

// trimWindowsRoot reports a slashed Windows path relative to root, matching
// the root without regard to case and only on a directory boundary.
//
// Parameters:
//   - path: A Windows path with forward slashes.
//   - root: The Plex-side root with forward slashes.
//
// Returns:
//   - rel: The remainder after root.
//   - ok: True when path is root or a descendant of root.
func trimWindowsRoot(path, root string) (string, bool) {
	root = strings.TrimRight(root, slash)
	if strings.EqualFold(path, root) {
		return "", true
	}

	prefix := root + slash
	if len(path) <= len(prefix) || !strings.EqualFold(path[:len(prefix)], prefix) {
		return "", false
	}

	return path[len(prefix):], true
}

// trimMediaRoot reports the path relative to root when path is root or a child of it.
//
// A string prefix is not enough: /data/media must not claim /data/media-other.
// The plex path is not cleaned first, because cleaning would resolve .. before
// the boundary check and hide the escape.
//
// Parameters:
//   - path: Path as the Plex server reports it.
//   - root: Plex-side root the path has to sit under.
//
// Returns:
//   - rel: The remainder after root, which may still contain .. segments.
//   - ok: True when path is root or a descendant of root.
func trimMediaRoot(path, root string) (string, bool) {
	root = strings.TrimRight(filepath.Clean(root), string(filepath.Separator))
	if path == root {
		return "", true
	}

	prefix := root + string(filepath.Separator)
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}

	return strings.TrimPrefix(path, prefix), true
}

// ensureDirs ensures the required directories exist.
//
// Parameters:
//   - cfg: Configuration naming the directories.
//
// Returns:
//   - err: Wrapped error naming the directory that could not be created.
func ensureDirs(cfg *Config) error {
	dirs := []string{
		filepath.Dir(cfg.DatabasePath),
		cfg.StoragePath,
	}
	for _, dir := range dirs {
		err := os.MkdirAll(dir, defaultDirPerms)
		if err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}

	return nil
}

// fileExists checks if a file exists.
//
// Parameters:
//   - path: Path to stat.
//
// Returns:
//   - exists: True when the path can be stat'd.
func fileExists(path string) bool {
	_, err := os.Stat(path)

	return err == nil
}

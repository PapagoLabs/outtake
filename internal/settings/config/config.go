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
	"strconv"
	"strings"
	"time"

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
	// PlexMediaRoot is the media path prefix as reported by Plex.
	PlexMediaRoot string `mapstructure:"plex-media-root"`
	// LocalMediaRoot is the local path prefix that replaces PlexMediaRoot.
	LocalMediaRoot string `mapstructure:"local-media-root"`
}

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
	viperInstance.SetDefault("log-level", defaultLogLevel)
	viperInstance.SetDefault("env", defaultEnv)
	viperInstance.SetDefault("session-poll-sec", int(defaultSessionPoll.Seconds()))
	viperInstance.SetDefault("num-workers", defaultNumWorkers)
	viperInstance.SetDefault("max-concurrent-previews", defaultMaxConcurrentPreviews)
	viperInstance.SetDefault("max-clip-dur", int(clip.MaxDuration.Seconds()))
	viperInstance.SetDefault("crop-black-bars", false)
	viperInstance.SetDefault("preserve-hdr", false)
	viperInstance.SetDefault("web-safe-color", false)
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
// into a time.Duration counts them as nanoseconds.
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

		seconds, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
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

	rel := plexPath
	if cfg.PlexMediaRoot != "" {
		rooted, ok := trimMediaRoot(plexPath, cfg.PlexMediaRoot)
		if !ok {
			return plexPath
		}

		rel = rooted
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

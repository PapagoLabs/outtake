// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package config provides configuration loading for outtake.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/adrg/xdg"
	"github.com/spf13/viper"
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
	// SessionPollSec is the session poll interval in seconds.
	SessionPollSec int `mapstructure:"session-poll-sec"`
	// NumWorkers is the number of workers.
	NumWorkers int `mapstructure:"num-workers"`
	// MaxClipDurSec is the maximum clip duration in seconds.
	MaxClipDurSec int `mapstructure:"max-clip-dur-sec"`
	// CropBlackBars is the default for trimming letterbox/pillarbox bars.
	CropBlackBars bool `mapstructure:"crop-black-bars"`
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
	// AppName is the application name.
	appName = "outtake"
	// DefaultListenAddr is the default listen address.
	defaultListenAddr = "0.0.0.0:8080"
	// DefaultLogLevel is the default log level.
	defaultLogLevel = "info"
	// DefaultEnv is the default environment.
	defaultEnv = "production"
	// DefaultFFmpegPath is the default FFmpeg path.
	defaultFFmpegPath = "ffmpeg"
	// DefaultFFprobePath is the default FFprobe path.
	defaultFFprobePath = "ffprobe"
	// DefaultSessionPoll is the default session poll interval in seconds.
	defaultSessionPoll = 10
	// DefaultNumWorkers is the default number of workers.
	defaultNumWorkers = 2
	// DefaultMaxClipDur is the default maximum clip duration in seconds.
	defaultMaxClipDur = 600
	// DefaultDirPerms is the default directory permissions.
	defaultDirPerms = 0o755
	// DefaultDatabaseBackend is the default database backend.
	defaultDatabaseBackend = "sqlite"
	// DefaultStorageBackend is the default storage backend.
	defaultStorageBackend = "filesystem"
	// DefaultS3Region is the default S3 region.
	defaultS3Region = "us-east-1"
)

// ConfigPath returns the configuration file path.
func ConfigPath() string {
	return filepath.Join(xdg.ConfigHome, appName, "config.yaml")
}

// DatabasePath returns the database file path.
func DatabasePath() string {
	return filepath.Join(xdg.DataHome, appName, "outtake.db")
}

// StoragePath returns the storage directory path.
func StoragePath() string {
	return filepath.Join(xdg.DataHome, appName, "output")
}

// Load loads the configuration.
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

	err = viperInstance.Unmarshal(cfg)
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
		SessionPollSec:  0,
		NumWorkers:      0,
		MaxClipDurSec:   0,
		CropBlackBars:   false,
		PlexServerURL:   "",
		PlexToken:       "",
		PlexClientID:    "",
		PublicBaseURL:   "",
		PlexMediaRoot:   "",
		LocalMediaRoot:  "",
	}
}

// setDefaults sets the default configuration values.
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
	viperInstance.SetDefault("session-poll-sec", defaultSessionPoll)
	viperInstance.SetDefault("num-workers", defaultNumWorkers)
	viperInstance.SetDefault("max-clip-dur-sec", defaultMaxClipDur)
	viperInstance.SetDefault("crop-black-bars", false)
	viperInstance.SetDefault("plex-media-root", "")
	viperInstance.SetDefault("local-media-root", "")
}

// bindEnv registers OUTTAKE_* environment keys so Unmarshal can see them.
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
		"max-clip-dur-sec",
		"crop-black-bars",
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
// When only LocalMediaRoot is set, the Plex path is joined under that mount.
// That covers servers that report library roots such as /Movies or /TV.
func (cfg *Config) RemapMediaPath(plexPath string) string {
	if cfg.LocalMediaRoot == "" {
		return plexPath
	}

	rel := plexPath
	if cfg.PlexMediaRoot != "" {
		if !strings.HasPrefix(plexPath, cfg.PlexMediaRoot) {
			return plexPath
		}

		rel = strings.TrimPrefix(plexPath, cfg.PlexMediaRoot)
	}

	rel = strings.TrimPrefix(rel, string(filepath.Separator))
	if rel == "" {
		return cfg.LocalMediaRoot
	}

	return filepath.Join(cfg.LocalMediaRoot, rel)
}

// ensureDirs ensures the required directories exist.
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
func fileExists(path string) bool {
	_, err := os.Stat(path)

	return err == nil
}

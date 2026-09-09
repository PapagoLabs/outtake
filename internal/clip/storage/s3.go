// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"

	"github.com/PapagoLabs/outtake/internal/config"
)

// S3 stores objects on an S3-compatible endpoint.
type S3 struct {
	fs     *Storage
	client *s3.Client
	bucket string
}

// s3Settings holds the values needed to construct an S3 backend.
type s3Settings struct {
	endpoint     string
	bucket       string
	region       string
	accessKey    string
	secretKey    string
	scratch      string
	usePathStyle bool
}

const (
	// getObjectErrFmt wraps S3 Get failures.
	getObjectErrFmt = "get object: %w"
)

var (
	// errPathOutsideScratch is returned when a path is outside the scratch dir.
	errPathOutsideScratch      = errors.New("path is outside storage scratch directory")
	_                     Blob = (*S3)(nil)
)

// newS3FromConfig builds an S3 backend from application configuration.
//
// Parameters:
//   - cfg: Application configuration.
//
// Returns:
//   - s3: An S3 backend from application configuration.
//   - err: The error, if any.
func newS3FromConfig(cfg *config.Config) (*S3, error) {
	if cfg.S3Bucket == "" {
		return nil, errS3BucketRequired
	}

	if cfg.S3Endpoint == "" {
		return nil, errS3EndpointRequired
	}

	region := cfg.S3Region
	if region == "" {
		region = defaultS3Region
	}

	store, err := newS3(s3Settings{
		endpoint:     cfg.S3Endpoint,
		bucket:       cfg.S3Bucket,
		region:       region,
		accessKey:    cfg.S3AccessKey,
		secretKey:    cfg.S3SecretKey,
		scratch:      cfg.StoragePath,
		usePathStyle: cfg.S3UsePathStyle,
	})
	if err != nil {
		return nil, fmt.Errorf("new s3: %w", err)
	}

	return store, nil
}

// newS3 constructs an S3 backend with a filesystem scratch directory.
//
// Parameters:
//   - settings: Settings.
//
// Returns:
//   - s3: An S3 backend with a filesystem scratch directory.
//   - err: The error, if any.
func newS3(settings s3Settings) (*S3, error) {
	fsStore, err := NewStorage(settings.scratch)
	if err != nil {
		return nil, fmt.Errorf("create scratch dir: %w", err)
	}

	client := s3.NewFromConfig(aws.Config{
		Region: settings.region,
		Credentials: aws.NewCredentialsCache(
			credentials.NewStaticCredentialsProvider(settings.accessKey, settings.secretKey, ""),
		),
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	}, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(settings.endpoint)
		options.UsePathStyle = settings.usePathStyle
		options.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		options.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
		if strings.HasPrefix(settings.endpoint, "http://") {
			options.EndpointOptions.DisableHTTPS = true
		}
	})

	return &S3{
		fs:     fsStore,
		client: client,
		bucket: settings.bucket,
	}, nil
}

// ClipPath returns the local scratch path for a clip file.
//
// Parameters:
//   - id: Identifier.
//
// Returns:
//   - value: The local scratch path for a clip file.
func (store *S3) ClipPath(id string) string {
	return store.fs.ClipPath(id)
}

// DeleteFile removes the object from S3 and the local scratch copy.
//
// Parameters:
//   - path: Filesystem path.
//
// Returns:
//   - err: The error, if any.
func (store *S3) DeleteFile(path string) error {
	key, err := store.objectKey(path)
	if err != nil {
		return fmt.Errorf("delete file: %w", err)
	}

	_, err = store.client.DeleteObject(context.Background(), &s3.DeleteObjectInput{
		Bucket: aws.String(store.bucket),
		Key:    aws.String(key),
	})
	if err != nil && !isNotFound(err) {
		return fmt.Errorf("delete object: %w", err)
	}

	err = store.fs.DeleteFile(path)
	if err != nil {
		return fmt.Errorf("delete local file: %w", err)
	}

	return nil
}

// FileExists reports whether the object exists, hydrating the local copy when
// needed.
//
// Parameters:
//   - path: Filesystem path.
//
// Returns:
//   - ok: True when the object exists, hydrating the local copy when needed.
func (store *S3) FileExists(path string) bool {
	if store.fs.FileExists(path) {
		return true
	}

	exists, err := store.head(context.Background(), path)
	if err != nil || !exists {
		return false
	}

	getErr := store.Get(context.Background(), path)

	return getErr == nil
}

// Get downloads the object from S3 onto the local scratch path.
//
// Parameters:
//   - ctx: Cancellation context.
//   - path: Filesystem path.
//
// Returns:
//   - err: The error, if any.
func (store *S3) Get(ctx context.Context, path string) error {
	key, err := store.objectKey(path)
	if err != nil {
		return fmt.Errorf(getObjectErrFmt, err)
	}

	output, err := store.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(store.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf(getObjectErrFmt, err)
	}
	defer output.Body.Close()

	err = writeObjectFile(path, output.Body)
	if err != nil {
		return fmt.Errorf(getObjectErrFmt, err)
	}

	return nil
}

// GifPath returns the local scratch path for a GIF file.
//
// Parameters:
//   - id: Identifier.
//
// Returns:
//   - value: The local scratch path for a GIF file.
func (store *S3) GifPath(id string) string {
	return store.fs.GifPath(id)
}

// PreviewPath returns the local scratch path for a preview file.
//
// Parameters:
//   - id: Identifier.
//
// Returns:
//   - value: The local scratch path for a preview file.
func (store *S3) PreviewPath(id string) string {
	return store.fs.PreviewPath(id)
}

// Put uploads the local scratch file to S3.
//
// Parameters:
//   - ctx: Cancellation context.
//   - path: Filesystem path.
//
// Returns:
//   - err: The error, if any.
func (store *S3) Put(ctx context.Context, path string) error {
	key, err := store.objectKey(path)
	if err != nil {
		return fmt.Errorf("put object: %w", err)
	}

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open local file: %w", err)
	}
	defer file.Close()

	_, err = store.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(store.bucket),
		Key:    aws.String(key),
		Body:   file,
	})
	if err != nil {
		return fmt.Errorf("put object: %w", err)
	}

	return nil
}

// ScreenshotPath returns the local scratch path for a screenshot file.
//
// Parameters:
//   - id: Identifier.
//
// Returns:
//   - value: The local scratch path for a screenshot file.
func (store *S3) ScreenshotPath(id string) string {
	return store.fs.ScreenshotPath(id)
}

// ThumbnailPath returns the local scratch path for a thumbnail file.
//
// Parameters:
//   - id: Identifier.
//
// Returns:
//   - value: The local scratch path for a thumbnail file.
func (store *S3) ThumbnailPath(id string) string {
	return store.fs.ThumbnailPath(id)
}

// WriteThumbnail stores thumbnail bytes locally and uploads them to S3.
//
// Parameters:
//   - id: Identifier.
//   - data: Data.
//
// Returns:
//   - err: The error, if any.
func (store *S3) WriteThumbnail(id string, data []byte) error {
	err := store.fs.WriteThumbnail(id, data)
	if err != nil {
		return fmt.Errorf("write thumbnail: %w", err)
	}

	err = store.Put(context.Background(), store.ThumbnailPath(id))
	if err != nil {
		return fmt.Errorf("upload thumbnail: %w", err)
	}

	return nil
}

// head reports whether an object exists on S3.
//
// Parameters:
//   - ctx: Cancellation context.
//   - path: Filesystem path.
//
// Returns:
//   - ok: True when an object exists on S3.
//   - err: The error, if any.
func (store *S3) head(ctx context.Context, path string) (bool, error) {
	key, err := store.objectKey(path)
	if err != nil {
		return false, fmt.Errorf("head object: %w", err)
	}

	_, err = store.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(store.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFound(err) {
			return false, nil
		}

		return false, fmt.Errorf("head object: %w", err)
	}

	return true, nil
}

// objectKey maps a local scratch path onto an S3 object key.
//
// Parameters:
//   - path: Filesystem path.
//
// Returns:
//   - value: A local scratch path onto an S3 object key.
//   - err: The error, if any.
func (store *S3) objectKey(path string) (string, error) {
	rel, err := filepath.Rel(store.fs.BasePath(), path)
	if err != nil {
		return "", fmt.Errorf("object key: %w", err)
	}

	if rel == "." || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("%w: %s", errPathOutsideScratch, path)
	}

	return filepath.ToSlash(rel), nil
}

// isNotFound reports whether err is an S3 missing-object error.
//
// Parameters:
//   - err: Error value.
//
// Returns:
//   - ok: True when err is an S3 missing-object error.
func isNotFound(err error) bool {
	apiErr, ok := errors.AsType[smithy.APIError](err)
	if ok {
		switch apiErr.ErrorCode() {
		case "NotFound", "NoSuchKey", "NoSuchBucket":
			return true
		}
	}

	var httpErr interface{ HTTPStatusCode() int }

	if errors.As(err, &httpErr) && httpErr.HTTPStatusCode() == http.StatusNotFound {
		return true
	}

	return false
}

// writeObjectFile writes downloaded object bytes to path.
//
// Parameters:
//   - path: Filesystem path.
//   - body: Body.
//
// Returns:
//   - err: The error, if any.
func writeObjectFile(path string, body io.Reader) error {
	dir := filepath.Dir(path)
	err := os.MkdirAll(dir, dirPermissions)
	if err != nil {
		return fmt.Errorf("create object dir: %w", err)
	}

	file, err := os.CreateTemp(dir, ".download-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	tmpPath := file.Name()

	_, copyErr := io.Copy(file, body)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)

		return fmt.Errorf("write local file: %w", copyErr)
	}

	if closeErr != nil {
		_ = os.Remove(tmpPath)

		return fmt.Errorf("close local file: %w", closeErr)
	}

	err = os.Rename(tmpPath, path)
	if err != nil {
		_ = os.Remove(tmpPath)

		return fmt.Errorf("rename local file: %w", err)
	}

	return nil
}

// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package blob

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

	"github.com/PapagoLabs/outtake/internal/settings/config"
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
	scratch      Paths
	usePathStyle bool
}

const (
	// getObjectErrFmt is the message S3 Get failures are wrapped with.
	getObjectErrFmt = "get object: %w"
	// defaultS3Region is the region used when none is configured.
	defaultS3Region = "us-east-1"
)

var (
	// errPathOutsideScratch reports a path outside the scratch directory.
	errPathOutsideScratch = errors.New("path is outside storage scratch directory")
	// errS3BucketRequired reports S3 selected without a bucket.
	errS3BucketRequired = errors.New("s3-bucket is required")
	// errS3EndpointRequired reports S3 selected without an endpoint.
	errS3EndpointRequired = errors.New("s3-endpoint is required")
)

// NewS3 constructs an S3 backend from application configuration.
//
// Parameters:
//   - cfg: App config carrying the bucket, endpoint, and credentials.
//   - paths: Local layout the backend scratches against.
//
// Returns:
//   - store: The S3 backend.
//   - err: Non-nil when the configuration is incomplete or the client cannot be built.
func NewS3(cfg *config.Config, paths Paths) (*S3, error) {
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

	store, err := newS3FromSettings(s3Settings{
		endpoint:     cfg.S3Endpoint,
		bucket:       cfg.S3Bucket,
		region:       region,
		accessKey:    cfg.S3AccessKey,
		secretKey:    cfg.S3SecretKey,
		scratch:      paths,
		usePathStyle: cfg.S3UsePathStyle,
	})
	if err != nil {
		return nil, fmt.Errorf("new s3: %w", err)
	}

	return store, nil
}

// newS3FromSettings constructs an S3 backend with a filesystem scratch directory.
//
// Parameters:
//   - settings: The endpoint, bucket, region, credentials, and scratch layout.
//
// Returns:
//   - store: The S3 backend.
//   - err: Non-nil when the scratch directory cannot be created.
func newS3FromSettings(settings s3Settings) (*S3, error) {
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

// DeleteFile removes the object from S3 and the local scratch copy.
//
// Parameters:
//   - path: The scratch path to remove from both copies.
//
// Returns:
//   - err: Non-nil when either copy cannot be removed.
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

// Ensure makes the object available on the local scratch path, downloading it
// only when no local copy exists.
//
// Parameters:
//   - ctx: Request scope for the download.
//   - path: The scratch path to fill.
//
// Returns:
//   - err: Non-nil when the object is not local and cannot be fetched.
func (store *S3) Ensure(ctx context.Context, path string) error {
	if store.fs.Exists(ctx, path) {
		return nil
	}

	err := store.fetch(ctx, path)
	if err != nil {
		return fmt.Errorf("ensure object: %w", err)
	}

	return nil
}

// Exists reports whether the object exists locally or on S3, asking S3 only
// for its headers.
//
// Parameters:
//   - ctx: Request scope for the lookup.
//   - path: The scratch path to check.
//
// Returns:
//   - exists: True when the object is present locally or on S3.
func (store *S3) Exists(ctx context.Context, path string) bool {
	if store.fs.Exists(ctx, path) {
		return true
	}

	exists, err := store.head(ctx, path)

	return err == nil && exists
}

// Paths returns the local layout the backend scratches against.
//
// Returns:
//   - paths: The backing filesystem layout.
func (store *S3) Paths() Paths {
	return store.fs.Paths
}

// Put uploads the local scratch file to S3.
//
// Parameters:
//   - ctx: Request scope for the upload.
//   - path: The scratch path to upload.
//
// Returns:
//   - err: Non-nil when the file cannot be opened or uploaded.
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

// WriteThumbnail stores thumbnail bytes locally and uploads them to S3.
//
// Parameters:
//   - id: Identifier the thumbnail is filed under.
//   - data: The encoded thumbnail bytes.
//
// Returns:
//   - err: Non-nil when the thumbnail cannot be written or uploaded.
func (store *S3) WriteThumbnail(id string, data []byte) error {
	err := store.fs.WriteThumbnail(id, data)
	if err != nil {
		return fmt.Errorf("write thumbnail: %w", err)
	}

	err = store.Put(context.Background(), store.fs.ThumbnailPath(id))
	if err != nil {
		return fmt.Errorf("upload thumbnail: %w", err)
	}

	return nil
}

// fetch downloads the object from S3 onto the local scratch path.
//
// Parameters:
//   - ctx: Request scope for the download.
//   - path: The scratch path to write.
//
// Returns:
//   - err: Non-nil when the object cannot be fetched or written.
func (store *S3) fetch(ctx context.Context, path string) error {
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

// head reports whether an object exists on S3.
//
// Parameters:
//   - ctx: Request scope for the lookup.
//   - path: The scratch path to look up.
//
// Returns:
//   - exists: True when S3 holds the object.
//   - err: Non-nil when the lookup fails for a reason other than absence.
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
//   - path: The scratch path to translate.
//
// Returns:
//   - key: The object key relative to the scratch directory.
//   - err: Non-nil when the path does not resolve inside the scratch directory.
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
//   - err: The error from an S3 request.
//
// Returns:
//   - missing: True when the object was absent.
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
//   - path: The destination path.
//   - body: The object bytes to write.
//
// Returns:
//   - err: Non-nil when the download cannot be staged or moved into place.
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

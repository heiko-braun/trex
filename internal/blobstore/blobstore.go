// Package blobstore is the resolver library over the object store
// (design doc section 6/9.1): a content-addressed put/get boundary that
// activities use to move data in and out of Minio. Agents and the
// workflow never call this package directly — only activity code does.
package blobstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/heiko-braun/trex/internal/manifest"
)

// Store puts and gets content-addressed blobs, scoped per tenant.
type Store interface {
	// Put uploads content under tenant, keyed by its SHA-256 digest, and
	// returns a Ref describing it. Uploading the same content twice is
	// idempotent: it returns the same digest without re-uploading.
	Put(ctx context.Context, tenant, mediaType string, content []byte) (manifest.Ref, error)

	// Get downloads the content described by ref from tenant.
	Get(ctx context.Context, tenant string, ref manifest.Ref) ([]byte, error)
}

// MinioStore is a Store backed by a Minio (or any S3-compatible) bucket,
// using key layout "{tenant}/sha256/{hash}" per design doc section 6.
type MinioStore struct {
	client *minio.Client
	bucket string
}

// Config holds the connection details for a local Minio instance.
type Config struct {
	Endpoint  string // e.g. "localhost:9000", no scheme
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

// NewMinioStore connects to Minio per cfg and ensures the target bucket
// exists, creating it if not (mirroring make db's self-contained
// startup: no separate provisioning step required).
func NewMinioStore(ctx context.Context, cfg Config) (*MinioStore, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}

	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("check bucket %q: %w", cfg.Bucket, err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("create bucket %q: %w", cfg.Bucket, err)
		}
	}

	return &MinioStore{client: client, bucket: cfg.Bucket}, nil
}

// Put implements Store.
func (s *MinioStore) Put(ctx context.Context, tenant, mediaType string, content []byte) (manifest.Ref, error) {
	sum := sha256.Sum256(content)
	digest := "sha256:" + hex.EncodeToString(sum[:])
	key := objectKey(tenant, digest)

	_, err := s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(content), int64(len(content)),
		minio.PutObjectOptions{ContentType: mediaType})
	if err != nil {
		return manifest.Ref{}, fmt.Errorf("put object %q: %w", key, err)
	}

	return manifest.Ref{
		MediaType: mediaType,
		Digest:    digest,
		Size:      int64(len(content)),
		Bucket:    s.bucket,
	}, nil
}

// Get implements Store. It reads from ref.Bucket when set (a
// self-locating ref, per section 5.1's "urls" field), falling back to
// this store's own bucket for refs produced before that field existed.
func (s *MinioStore) Get(ctx context.Context, tenant string, ref manifest.Ref) ([]byte, error) {
	bucket := ref.Bucket
	if bucket == "" {
		bucket = s.bucket
	}
	key := objectKey(tenant, ref.Digest)

	obj, err := s.client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get object %q: %w", key, err)
	}
	defer obj.Close()

	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, fmt.Errorf("read object %q: %w", key, err)
	}
	return data, nil
}

// objectKey builds the content-addressed key "{tenant}/sha256/{hash}"
// from a "sha256:<hex>" digest.
func objectKey(tenant, digest string) string {
	const prefix = "sha256:"
	hash := digest
	if len(digest) > len(prefix) && digest[:len(prefix)] == prefix {
		hash = digest[len(prefix):]
	}
	return fmt.Sprintf("%s/sha256/%s", tenant, hash)
}

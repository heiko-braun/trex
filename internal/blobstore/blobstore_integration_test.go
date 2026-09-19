//go:build integration

package blobstore

import (
	"context"
	"os"
	"testing"

	"github.com/heiko-braun/trex/internal/manifest"
)

// newTestStore connects to a local Minio instance started via `make
// minio`, using a bucket unique to the test to avoid cross-test
// collisions. Skips if MINIO_ENDPOINT isn't set, mirroring
// store/postgres's TEST_DATABASE_URL convention.
func newTestStore(t *testing.T) *MinioStore {
	t.Helper()

	endpoint := os.Getenv("MINIO_ENDPOINT")
	if endpoint == "" {
		t.Skip("MINIO_ENDPOINT not set; run `make minio` and set MINIO_ENDPOINT=localhost:9000 to run this test")
	}

	store, err := NewMinioStore(context.Background(), Config{
		Endpoint:  endpoint,
		AccessKey: envOr("MINIO_ACCESS_KEY", "trex"),
		SecretKey: envOr("MINIO_SECRET_KEY", "trex12345"),
		Bucket:    "trex-test",
	})
	if err != nil {
		t.Fatalf("NewMinioStore: %v", err)
	}
	return store
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func TestMinioStore_PutThenGet_RoundTrips(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	content := []byte(`{"trend":"stable"}`)

	ref, err := store.Put(ctx, "platform", "application/json", content)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if ref.Size != int64(len(content)) {
		t.Errorf("Size = %d, want %d", ref.Size, len(content))
	}
	if ref.MediaType != "application/json" {
		t.Errorf("MediaType = %q, want application/json", ref.MediaType)
	}
	if ref.Bucket != "trex-test" {
		t.Errorf("Bucket = %q, want %q (ref must be self-locating)", ref.Bucket, "trex-test")
	}

	got, err := store.Get(ctx, "platform", ref)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("Get = %q, want %q", got, content)
	}
}

func TestMinioStore_Put_IsContentAddressedAndIdempotent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	content := []byte("same content twice")

	ref1, err := store.Put(ctx, "platform", "text/plain", content)
	if err != nil {
		t.Fatalf("first Put: %v", err)
	}
	ref2, err := store.Put(ctx, "platform", "text/plain", content)
	if err != nil {
		t.Fatalf("second Put: %v", err)
	}

	if ref1.Digest != ref2.Digest {
		t.Errorf("digests differ for identical content: %q vs %q", ref1.Digest, ref2.Digest)
	}
}

func TestMinioStore_Get_DifferentTenantsAreIsolated(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	content := []byte("tenant-scoped content")

	ref, err := store.Put(ctx, "platform", "text/plain", content)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	var empty manifest.Ref
	if ref == empty {
		t.Fatal("Put returned zero Ref")
	}

	if _, err := store.Get(ctx, "other-tenant", ref); err == nil {
		t.Error("Get under a different tenant succeeded, want error (tenants must be isolated by key prefix)")
	}
}

func TestMinioStore_PutIndexThenGetIndex_RoundTrips(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	workflowID := "pod-memory-trend-test-1"
	slots := map[string]manifest.Ref{
		"opsBuddyRef":      {Digest: "sha256:aaa", Size: 3, MediaType: "text/plain", Bucket: "trex-test"},
		"logMonitoringRef": {Digest: "sha256:bbb", Size: 4, MediaType: "text/plain", Bucket: "trex-test"},
	}

	if err := store.PutIndex(ctx, "platform", workflowID, slots); err != nil {
		t.Fatalf("PutIndex: %v", err)
	}

	got, err := store.GetIndex(ctx, "platform", workflowID)
	if err != nil {
		t.Fatalf("GetIndex: %v", err)
	}
	if len(got) != len(slots) {
		t.Fatalf("GetIndex returned %d slots, want %d", len(got), len(slots))
	}
	for slot, want := range slots {
		if got[slot] != want {
			t.Errorf("slot %q = %+v, want %+v", slot, got[slot], want)
		}
	}
}

func TestMinioStore_PutIndex_OverwritesPreviousValue(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	workflowID := "pod-memory-trend-test-2"

	first := map[string]manifest.Ref{"opsBuddyRef": {Digest: "sha256:aaa"}}
	second := map[string]manifest.Ref{"opsBuddyRef": {Digest: "sha256:zzz"}, "editorRef": {Digest: "sha256:yyy"}}

	if err := store.PutIndex(ctx, "platform", workflowID, first); err != nil {
		t.Fatalf("first PutIndex: %v", err)
	}
	if err := store.PutIndex(ctx, "platform", workflowID, second); err != nil {
		t.Fatalf("second PutIndex: %v", err)
	}

	got, err := store.GetIndex(ctx, "platform", workflowID)
	if err != nil {
		t.Fatalf("GetIndex: %v", err)
	}
	if len(got) != 2 || got["opsBuddyRef"].Digest != "sha256:zzz" {
		t.Errorf("GetIndex = %+v, want the second PutIndex's value (overwritten, not merged)", got)
	}
}

func TestMinioStore_GetIndex_NotFoundForUnknownWorkflow(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if _, err := store.GetIndex(ctx, "platform", "never-existed"); err == nil {
		t.Error("GetIndex for an unknown workflow ID succeeded, want error")
	}
}

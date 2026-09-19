package main

import (
	"context"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"go.temporal.io/sdk/testsuite"

	"github.com/heiko-braun/trex/internal/manifest"
	"github.com/heiko-braun/trex/internal/workflowworker"
	"github.com/heiko-braun/trex/store"
)

// fakeBlobStore is a no-op BlobStore, standing in for
// *blobstore.MinioStore in tests that don't exercise envelope writing.
type fakeBlobStore struct{}

func (fakeBlobStore) PutIndex(_ context.Context, _, _ string, _ map[string]manifest.Ref) error {
	return nil
}

var devServer *testsuite.DevServer

func TestMain(m *testing.M) {
	temporalPath, err := exec.LookPath("temporal")
	if err != nil {
		panic("temporal CLI not found on PATH: " + err.Error())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	srv, err := testsuite.StartDevServer(ctx, testsuite.DevServerOptions{
		ExistingPath: temporalPath,
		Stdout:       io.Discard,
		Stderr:       io.Discard,
	})
	cancel()
	if err != nil {
		panic("start temporal dev server: " + err.Error())
	}
	devServer = srv
	defer func() { _ = devServer.Stop() }()

	os.Exit(m.Run())
}

// TestRestoreWorkflowWorker_RecoversFromZigflowPanic guards against a
// regression of the crash found running this server against a
// Postgres with stale/malformed stored definitions: zigflow.LoadFromBytes
// panics (rather than returning an error) on some malformed YAML, and one
// bad row must not take down the boot-time restore loop for every other
// definition.
func TestRestoreWorkflowWorker_RecoversFromZigflowPanic(t *testing.T) {
	supervisor := workflowworker.NewSupervisor(devServer.Client(), fakeBlobStore{})
	defer supervisor.Stop()

	def := &store.Definition{Tenant: "acme", Name: "broken", YAML: "document: {}"}

	ok := restoreWorkflowWorker(supervisor, def)
	if ok {
		t.Fatal("restoreWorkflowWorker returned true for a definition known to panic zigflow.LoadFromBytes")
	}
	if len(supervisor.Statuses()) != 0 {
		t.Fatalf("Statuses() = %d entries, want 0 after a recovered panic", len(supervisor.Statuses()))
	}
}

func TestRestoreWorkflowWorker_StartsWorkerForValidDefinition(t *testing.T) {
	supervisor := workflowworker.NewSupervisor(devServer.Client(), fakeBlobStore{})
	defer supervisor.Stop()

	def := &store.Definition{Tenant: "acme", Name: "valid", YAML: `
document:
  dsl: 1.0.0
  taskQueue: restore-test-queue
  workflowType: restore-test
  version: 0.0.1

do:
  - ping:
      call: http
      with:
        method: get
        endpoint: https://example.com
`}

	ok := restoreWorkflowWorker(supervisor, def)
	if !ok {
		t.Fatal("restoreWorkflowWorker returned false for a valid definition")
	}
	if len(supervisor.Statuses()) != 1 {
		t.Fatalf("Statuses() = %d entries, want 1", len(supervisor.Statuses()))
	}
}

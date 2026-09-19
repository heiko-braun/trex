package workflowworker

import (
	"context"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"go.temporal.io/sdk/testsuite"

	"github.com/heiko-braun/trex/internal/manifest"
)

// fakeBlobStore is an in-memory BlobStore, standing in for
// *blobstore.MinioStore in tests.
type fakeBlobStore struct{}

func (fakeBlobStore) PutIndex(_ context.Context, _, _ string, _ map[string]manifest.Ref) error {
	return nil
}

// devServer is a single embedded Temporal dev server shared by every
// test in this package: worker.Start() connects eagerly, so bookkeeping
// tests need a real (if ephemeral) server rather than a live-connection
// mock.
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

const validWorkflowYAML = `
document:
  dsl: 1.0.0
  taskQueue: test-queue
  workflowType: test-workflow
  version: 0.0.1

do:
  - ping:
      call: http
      with:
        method: get
        endpoint: https://example.com
`

const otherQueueWorkflowYAML = `
document:
  dsl: 1.0.0
  taskQueue: other-queue
  workflowType: test-workflow
  version: 0.0.2

do:
  - ping:
      call: http
      with:
        method: get
        endpoint: https://example.com
`

func newTestSupervisor(t *testing.T) *Supervisor {
	t.Helper()
	return NewSupervisor(devServer.Client(), fakeBlobStore{})
}

func TestSupervisor_Register_StartsWorker(t *testing.T) {
	s := newTestSupervisor(t)
	defer s.Stop()

	if err := s.Register("acme", "wf", "test-queue", []byte(validWorkflowYAML)); err != nil {
		t.Fatalf("Register: %v", err)
	}

	statuses := s.Statuses()
	if len(statuses) != 1 {
		t.Fatalf("Statuses() = %d entries, want 1", len(statuses))
	}
	if !statuses[0].Running || statuses[0].TaskQueue != "test-queue" {
		t.Errorf("status = %+v, want running=true taskQueue=test-queue", statuses[0])
	}
}

func TestSupervisor_Unregister_StopsWorker(t *testing.T) {
	s := newTestSupervisor(t)
	defer s.Stop()

	if err := s.Register("acme", "wf", "test-queue", []byte(validWorkflowYAML)); err != nil {
		t.Fatalf("Register: %v", err)
	}
	s.Unregister("acme", "wf")

	if len(s.Statuses()) != 0 {
		t.Fatalf("Statuses() = %d entries, want 0 after unregister", len(s.Statuses()))
	}
}

func TestSupervisor_Register_ReplacesRunningWorkerOnRepublish(t *testing.T) {
	s := newTestSupervisor(t)
	defer s.Stop()

	if err := s.Register("acme", "wf", "test-queue", []byte(validWorkflowYAML)); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := s.Register("acme", "wf", "other-queue", []byte(otherQueueWorkflowYAML)); err != nil {
		t.Fatalf("second Register: %v", err)
	}

	statuses := s.Statuses()
	if len(statuses) != 1 {
		t.Fatalf("Statuses() = %d entries, want 1 (republish replaces, not adds)", len(statuses))
	}
	if statuses[0].TaskQueue != "other-queue" {
		t.Errorf("TaskQueue = %q, want %q (the republished revision's queue)", statuses[0].TaskQueue, "other-queue")
	}
}

func TestSupervisor_Register_InvalidYAMLReturnsError(t *testing.T) {
	s := newTestSupervisor(t)
	defer s.Stop()

	if err := s.Register("acme", "bad", "test-queue", []byte("not valid")); err == nil {
		t.Fatal("Register: expected error for invalid YAML, got nil")
	}
	if len(s.Statuses()) != 0 {
		t.Fatalf("Statuses() = %d entries, want 0 after failed Register", len(s.Statuses()))
	}
}

func TestSupervisor_Unregister_UnknownIsNoop(t *testing.T) {
	s := newTestSupervisor(t)
	defer s.Stop()

	s.Unregister("acme", "never-registered")

	if len(s.Statuses()) != 0 {
		t.Fatalf("Statuses() = %d entries, want 0", len(s.Statuses()))
	}
}

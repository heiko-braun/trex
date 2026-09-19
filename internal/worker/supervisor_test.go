package worker

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"go.temporal.io/sdk/testsuite"

	"github.com/heiko-braun/trex/internal/agents"
	"github.com/heiko-braun/trex/internal/manifest"
)

// devServer is a single embedded Temporal dev server shared by every test
// in this package: worker.Start() connects eagerly, so bookkeeping tests
// need a real (if ephemeral) server rather than a live-connection mock.
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

// fakeDispatcher records dispatch calls; never actually invoked by these
// reconcile-focused tests since no workflow tasks are polled.
type fakeDispatcher struct {
	mu    sync.Mutex
	calls int
}

func (f *fakeDispatcher) Dispatch(_ context.Context, _, _, _ string) (agents.DispatchResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return agents.DispatchResult{Done: true, Result: "ok"}, nil
}

func (f *fakeDispatcher) PollTask(_ context.Context, _, _, _ string) (agents.TaskStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return agents.TaskStatus{Done: true, Result: "ok"}, nil
}

// fakeTokenSource returns a fixed token, standing in for
// *agents.AgentctlTokenSource in tests.
type fakeTokenSource struct{}

func (fakeTokenSource) Token(_ context.Context) (string, error) {
	return "fake-token", nil
}

// fakeBlobStore is an in-memory BlobStore, standing in for
// *blobstore.MinioStore in tests.
type fakeBlobStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newFakeBlobStore() *fakeBlobStore {
	return &fakeBlobStore{data: map[string][]byte{}}
}

func (f *fakeBlobStore) Put(_ context.Context, tenant, mediaType string, content []byte) (manifest.Ref, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	digest := fmt.Sprintf("sha256:fake-%d", len(f.data))
	f.data[tenant+"/"+digest] = content
	return manifest.Ref{MediaType: mediaType, Digest: digest, Size: int64(len(content))}, nil
}

func (f *fakeBlobStore) Get(_ context.Context, tenant string, ref manifest.Ref) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	content, ok := f.data[tenant+"/"+ref.Digest]
	if !ok {
		return nil, fmt.Errorf("no such blob: %s/%s", tenant, ref.Digest)
	}
	return content, nil
}

func newTestSupervisor(t *testing.T) *Supervisor {
	t.Helper()
	return NewSupervisor(devServer.Client(), &fakeDispatcher{}, fakeTokenSource{}, newFakeBlobStore())
}

func TestSupervisor_Register_StartsWorker(t *testing.T) {
	s := newTestSupervisor(t)
	defer s.Stop()

	agent := agents.Agent{ID: "agent-1", Name: "test-agent", Enabled: true}
	if err := s.Register(agent); err != nil {
		t.Fatalf("Register: %v", err)
	}

	statuses := s.Statuses()
	if len(statuses) != 1 {
		t.Fatalf("Statuses() = %d entries, want 1", len(statuses))
	}
	if !statuses[0].Running {
		t.Errorf("agent-1 worker not running")
	}
	if statuses[0].TaskQueue != "agent-agent-1" {
		t.Errorf("TaskQueue = %q, want %q", statuses[0].TaskQueue, "agent-agent-1")
	}
}

func TestSupervisor_Unregister_StopsWorker(t *testing.T) {
	s := newTestSupervisor(t)
	defer s.Stop()

	agent := agents.Agent{ID: "agent-1", Name: "test-agent", Enabled: true}
	if err := s.Register(agent); err != nil {
		t.Fatalf("Register: %v", err)
	}
	s.Unregister(agent.Slug())

	statuses := s.Statuses()
	if len(statuses) != 0 {
		t.Fatalf("Statuses() = %d entries, want 0 after unregister", len(statuses))
	}
}

func TestSupervisor_Register_IsIdempotent(t *testing.T) {
	s := newTestSupervisor(t)
	defer s.Stop()

	agent := agents.Agent{ID: "agent-1", Name: "test-agent", Enabled: true}
	if err := s.Register(agent); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := s.Register(agent); err != nil {
		t.Fatalf("second Register: %v", err)
	}

	statuses := s.Statuses()
	if len(statuses) != 1 {
		t.Fatalf("Statuses() = %d entries, want 1 (duplicate register should be a no-op)", len(statuses))
	}
}

func TestSupervisor_Unregister_UnknownAgentIsNoop(t *testing.T) {
	s := newTestSupervisor(t)
	defer s.Stop()

	s.Unregister("never-registered")

	if len(s.Statuses()) != 0 {
		t.Fatalf("Statuses() = %d entries, want 0", len(s.Statuses()))
	}
}

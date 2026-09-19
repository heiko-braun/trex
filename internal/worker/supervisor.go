// Package worker starts and stops Temporal workers for explicitly
// registered agents: one worker per registered agent, polling that
// agent's own task queue, with a single activity that dispatches the call
// via A2A. See specs/agent-discovery-and-registration.md.
package worker

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"github.com/heiko-braun/trex/internal/agents"
	"github.com/heiko-braun/trex/internal/manifest"
)

// Dispatcher sends messages to agents via A2A and checks on their task
// status, one HTTP call at a time — neither call blocks waiting for a
// task to finish. Implemented by *agents.Dispatcher; abstracted here for
// testing.
type Dispatcher interface {
	Dispatch(ctx context.Context, agentID, token, input string) (agents.DispatchResult, error)
	PollTask(ctx context.Context, agentID, token, taskID string) (agents.TaskStatus, error)
}

// TokenSource supplies a fresh Bearer token for each A2A call, fetched
// right before the call rather than passed in by the workflow.
// Implemented by *agents.AgentctlTokenSource; abstracted here for
// testing.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// BlobStore resolves and stores content-addressed blobs for activities,
// per docs/architecure/task-envelope-design.md section 9.1, plus the
// per-workflow index used by the envelope browser (see
// specs/task-envelope-browser.md). Implemented by *blobstore.MinioStore;
// abstracted here for testing.
type BlobStore interface {
	Put(ctx context.Context, tenant, mediaType string, content []byte) (manifest.Ref, error)
	Get(ctx context.Context, tenant string, ref manifest.Ref) ([]byte, error)
	PutIndex(ctx context.Context, tenant, workflowID string, slots map[string]manifest.Ref) error
	GetIndex(ctx context.Context, tenant, workflowID string) (map[string]manifest.Ref, error)
}

// Status reports whether a registered agent's worker is currently
// running, for the read-only registration list.
type Status struct {
	Agent     agents.Agent
	TaskQueue string
	Running   bool
}

// Supervisor starts and stops one Temporal worker per explicitly
// registered agent, keyed by agent slug. Registration is driven directly
// by API calls (Register/Unregister), not by a background poller: there
// is no discovery diff to reconcile against, only persisted registration
// state loaded once at startup and mutated on demand.
type Supervisor struct {
	client     client.Client
	dispatcher Dispatcher
	tokens     TokenSource
	blobs      BlobStore

	mu      sync.Mutex
	workers map[string]worker.Worker
	known   map[string]agents.Agent
}

// NewSupervisor builds a Supervisor that starts workers against the given
// Temporal client, dispatching activity calls through dispatcher,
// fetching a fresh token from tokens right before each call, and
// resolving/storing agent input and results through blobs.
func NewSupervisor(c client.Client, dispatcher Dispatcher, tokens TokenSource, blobs BlobStore) *Supervisor {
	return &Supervisor{
		client:     c,
		dispatcher: dispatcher,
		tokens:     tokens,
		blobs:      blobs,
		workers:    map[string]worker.Worker{},
		known:      map[string]agents.Agent{},
	}
}

// Register starts a Temporal worker for agent on its own task queue.
// Registering an already-registered agent is a no-op.
func (s *Supervisor) Register(agent agents.Agent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	slug := agent.Slug()
	if _, running := s.workers[slug]; running {
		return nil
	}

	w := worker.New(s.client, agent.TaskQueue(), worker.Options{})
	w.RegisterActivityWithOptions(s.invokeAgentActivity(agent), activity.RegisterOptions{
		Name: "invoke-agent",
	})
	w.RegisterActivityWithOptions(s.pollAgentTaskActivity(agent), activity.RegisterOptions{
		Name: "poll-agent-task",
	})

	if err := w.Start(); err != nil {
		return fmt.Errorf("start worker for agent %s: %w", slug, err)
	}

	s.workers[slug] = w
	s.known[slug] = agent
	slog.Info("agent worker started", "agent", slug, "taskQueue", agent.TaskQueue())
	return nil
}

// Unregister stops the Temporal worker for the agent with the given
// slug. Unregistering an agent that isn't registered is a no-op.
func (s *Supervisor) Unregister(slug string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	w, running := s.workers[slug]
	if !running {
		return
	}
	w.Stop()
	delete(s.workers, slug)
	delete(s.known, slug)
	slog.Info("agent worker stopped", "agent", slug)
}

// Statuses returns the current per-agent worker status for the read-only
// registration list API.
func (s *Supervisor) Statuses() []Status {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]Status, 0, len(s.known))
	for slug, agent := range s.known {
		_, running := s.workers[slug]
		out = append(out, Status{Agent: agent, TaskQueue: agent.TaskQueue(), Running: running})
	}
	return out
}

// Stop stops every running agent worker, e.g. on server shutdown.
func (s *Supervisor) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for slug, w := range s.workers {
		w.Stop()
		delete(s.workers, slug)
	}
}

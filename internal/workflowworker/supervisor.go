// Package workflowworker starts and stops Temporal workers for published
// Zigflow workflow definitions: one worker per (tenant, name), polling
// that definition's own task queue. Unversioned first pass — see
// specs/workflow-worker-supervisor.md; Build ID / Worker Deployment
// versioning and the rollout coordinator are follow-up specs.
package workflowworker

import (
	"fmt"
	"log/slog"
	"sync"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"github.com/heiko-braun/trex/internal/zigflowadapter"
)

// key identifies one workflow worker by tenant and logical name,
// independent of which YAML revision (Build ID) is currently running on
// it — this spec runs whatever was most recently published,
// unconditionally.
type key struct {
	tenant string
	name   string
}

// Status reports a running workflow worker's identity and task queue,
// for the read-only definitions view.
type Status struct {
	Tenant    string
	Name      string
	TaskQueue string
	Running   bool
}

// Supervisor starts and stops one Temporal worker per published
// (tenant, name) workflow definition.
type Supervisor struct {
	client client.Client

	mu      sync.Mutex
	workers map[key]worker.Worker
	queues  map[key]string
}

// NewSupervisor builds a Supervisor that starts workers against the
// given Temporal client.
func NewSupervisor(c client.Client) *Supervisor {
	return &Supervisor{
		client:  c,
		workers: map[key]worker.Worker{},
		queues:  map[key]string{},
	}
}

// Register builds yamlBytes' closure tree and starts a worker for it on
// its own task queue (document.taskQueue in the YAML). If (tenant, name)
// already has a running worker, that worker is stopped first: publishing
// a new revision replaces the running definition rather than running both.
func (s *Supervisor) Register(tenant, name, taskQueue string, yamlBytes []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	k := key{tenant: tenant, name: name}
	s.stopLocked(k)

	w := worker.New(s.client, taskQueue, worker.Options{})
	if err := zigflowadapter.Build(w, yamlBytes); err != nil {
		return fmt.Errorf("build workflow %s/%s: %w", tenant, name, err)
	}

	if err := w.Start(); err != nil {
		return fmt.Errorf("start worker for workflow %s/%s: %w", tenant, name, err)
	}

	s.workers[k] = w
	s.queues[k] = taskQueue
	slog.Info("workflow worker started", "tenant", tenant, "name", name, "taskQueue", taskQueue)
	return nil
}

// Unregister stops the worker for (tenant, name), if any.
func (s *Supervisor) Unregister(tenant, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopLocked(key{tenant: tenant, name: name})
}

func (s *Supervisor) stopLocked(k key) {
	w, running := s.workers[k]
	if !running {
		return
	}
	w.Stop()
	delete(s.workers, k)
	delete(s.queues, k)
	slog.Info("workflow worker stopped", "tenant", k.tenant, "name", k.name)
}

// Statuses returns the current per-definition worker status.
func (s *Supervisor) Statuses() []Status {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]Status, 0, len(s.workers))
	for k, taskQueue := range s.queues {
		_, running := s.workers[k]
		out = append(out, Status{Tenant: k.tenant, Name: k.name, TaskQueue: taskQueue, Running: running})
	}
	return out
}

// Stop stops every running workflow worker, e.g. on server shutdown.
func (s *Supervisor) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, w := range s.workers {
		w.Stop()
		delete(s.workers, k)
		delete(s.queues, k)
	}
}

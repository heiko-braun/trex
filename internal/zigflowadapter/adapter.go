// Package zigflowadapter is the thin seam over Zigflow's Go API described
// in docs/architecure/zigflow-workflow-server-architecture.md:
// Build(yaml) -> register(worker). It isolates the rest of the server from
// Zigflow's internal packages so an upstream API change only touches this
// file. See specs/workflow-worker-supervisor.md.
package zigflowadapter

import (
	"fmt"

	"github.com/zigflow/zigflow/pkg/cloudevents"
	"github.com/zigflow/zigflow/pkg/telemetry"
	"github.com/zigflow/zigflow/pkg/zigflow"
	"github.com/zigflow/zigflow/pkg/zigflow/tasks"
	"go.temporal.io/sdk/worker"
)

// Build validates and parses yamlBytes, then registers the resulting
// workflow's closure tree onto temporalWorker so it can execute
// definition's workflow type when the worker is started.
func Build(temporalWorker worker.Worker, yamlBytes []byte) error {
	if err := zigflow.ValidateBytes(yamlBytes); err != nil {
		return fmt.Errorf("validate workflow: %w", err)
	}

	doc, err := zigflow.LoadFromBytes(yamlBytes)
	if err != nil {
		return fmt.Errorf("load workflow: %w", err)
	}

	// No CloudEvents sinks or telemetry reporting for server-hosted
	// workflow workers: an empty path gives a no-op Events emitter, and
	// disabled=true skips the anonymous telemetry heartbeat.
	emitter, err := cloudevents.Load("", nil, doc)
	if err != nil {
		return fmt.Errorf("build event emitter: %w", err)
	}
	telem, err := telemetry.New("", true)
	if err != nil {
		return fmt.Errorf("build telemetry: %w", err)
	}

	if err := zigflow.NewWorkflow(temporalWorker, doc, nil, emitter, telem, &tasks.TaskOpts{}); err != nil {
		return fmt.Errorf("register workflow: %w", err)
	}
	return nil
}

package zigflowadapter

import (
	"context"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/worker"
)

// devServer is a single embedded Temporal dev server shared by every
// test in this package: worker.New/Start connects eagerly, so testing
// Build's registration needs a real (if ephemeral) server.
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
  taskQueue: test
  workflowType: test
  version: 0.0.1

do:
  - ping:
      call: http
      with:
        method: get
        endpoint: https://example.com
`

func TestBuild_RegistersWorkflowOnWorker(t *testing.T) {
	w := worker.New(devServer.Client(), "test", worker.Options{})
	defer w.Stop()

	if err := Build(w, []byte(validWorkflowYAML)); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
}

func TestBuild_RejectsInvalidYAML(t *testing.T) {
	w := worker.New(devServer.Client(), "test-invalid", worker.Options{})
	defer w.Stop()

	if err := Build(w, []byte("not: valid: yaml: at: all:")); err == nil {
		t.Fatal("Build: expected error for invalid YAML, got nil")
	}
}

func TestBuild_RejectsSchemaInvalidWorkflow(t *testing.T) {
	w := worker.New(devServer.Client(), "test-schema-invalid", worker.Options{})
	defer w.Stop()

	// Missing required document.taskQueue/workflowType/version.
	missingFieldsYAML := `
document:
  dsl: 1.0.0

do:
  - ping:
      call: http
      with:
        method: get
        endpoint: https://example.com
`
	if err := Build(w, []byte(missingFieldsYAML)); err == nil {
		t.Fatal("Build: expected schema validation error, got nil")
	}
}

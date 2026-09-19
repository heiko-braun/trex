package workflowworker

import (
	"context"
	"testing"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"

	"github.com/heiko-braun/trex/internal/manifest"
)

// recordingBlobStore records the arguments PutIndex was called with, for
// verifying writeEnvelopeIndexActivity reads the calling workflow's own
// ID rather than something threaded through the input.
type recordingBlobStore struct {
	gotTenant, gotWorkflowID string
	gotSlots                 map[string]manifest.Ref
}

func (f *recordingBlobStore) PutIndex(_ context.Context, tenant, workflowID string, slots map[string]manifest.Ref) error {
	f.gotTenant, f.gotWorkflowID, f.gotSlots = tenant, workflowID, slots
	return nil
}

func TestWriteEnvelopeIndexActivity_UsesCallingWorkflowID(t *testing.T) {
	blobs := &recordingBlobStore{}
	slots := map[string]manifest.Ref{"opsBuddyRef": {Digest: "sha256:aaa", Size: 3}}

	callerWorkflow := func(ctx workflow.Context) error {
		ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{StartToCloseTimeout: 10 * time.Second})
		return workflow.ExecuteActivity(ctx, "write-envelope-index", WriteEnvelopeIndexInput{
			Tenant: "platform",
			Slots:  slots,
		}).Get(ctx, nil)
	}

	suite := testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterActivityWithOptions(writeEnvelopeIndexActivity(blobs), activity.RegisterOptions{Name: "write-envelope-index"})
	env.RegisterWorkflow(callerWorkflow)

	env.ExecuteWorkflow(callerWorkflow)

	if !env.IsWorkflowCompleted() {
		t.Fatalf("workflow did not complete")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error: %v", err)
	}

	if blobs.gotWorkflowID == "" {
		t.Errorf("PutIndex workflowID is empty, want the calling workflow's own ID")
	}
	if blobs.gotTenant != "platform" {
		t.Errorf("PutIndex tenant = %q, want %q", blobs.gotTenant, "platform")
	}
	if len(blobs.gotSlots) != 1 || blobs.gotSlots["opsBuddyRef"].Digest != "sha256:aaa" {
		t.Errorf("PutIndex slots = %+v, want the input slots unchanged", blobs.gotSlots)
	}
}

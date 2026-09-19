package workflowworker

import (
	"context"
	"fmt"

	"go.temporal.io/sdk/activity"

	"github.com/heiko-braun/trex/internal/manifest"
)

// BlobStore writes the per-workflow envelope index used by the envelope
// browser, per specs/task-envelope-browser.md. Implemented by
// *blobstore.MinioStore; abstracted here for testing.
type BlobStore interface {
	PutIndex(ctx context.Context, tenant, workflowID string, slots map[string]manifest.Ref) error
}

// WriteEnvelopeIndexInput is the Temporal activity input for recording a
// workflow execution's current slot refs so they can be looked up later
// by workflow ID. It must only ever be called from the root workflow's
// own do: list (never from inside a Zigflow for/while child workflow) —
// activity.GetInfo(ctx).WorkflowExecution.ID is the ID of whichever
// workflow directly invoked the activity, and Zigflow runs each for/while
// iteration as its own child workflow, so calling this from inside one
// would record the child's ID instead of the root's.
type WriteEnvelopeIndexInput struct {
	Tenant string
	Slots  map[string]manifest.Ref
}

// writeEnvelopeIndexActivity returns an activity function bound to blobs
// that writes in.Slots to the calling workflow's envelope index.
func writeEnvelopeIndexActivity(blobs BlobStore) func(ctx context.Context, in WriteEnvelopeIndexInput) error {
	return func(ctx context.Context, in WriteEnvelopeIndexInput) error {
		workflowID := activity.GetInfo(ctx).WorkflowExecution.ID
		if err := blobs.PutIndex(ctx, in.Tenant, workflowID, in.Slots); err != nil {
			return fmt.Errorf("write envelope index for workflow %s: %w", workflowID, err)
		}
		return nil
	}
}

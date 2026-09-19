package worker

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/heiko-braun/trex/internal/agents"
	"github.com/heiko-braun/trex/internal/manifest"
)

// tenant is hardcoded for this slice: no multi-tenant wiring exists yet
// anywhere in the workflow input or activity input. See
// docs/architecure/task-envelope-design.md and follow-up tickets for
// real tenant plumbing.
const tenant = "platform"

// InvokeAgentInput is the Temporal activity input for calling an agent.
// Instead of a single fully-composed Input string, it carries the refs
// the workflow has resolved for this call (Reads) plus a fixed
// Instruction template with "{{slot}}" placeholders; the activity
// resolves each ref from blobstore and substitutes its content into the
// template before dispatching. The A2A call's Bearer token is fetched
// fresh from the Supervisor's TokenSource right before the call, not
// passed in by the workflow — see specs/agent-discovery-and-registration.md's
// open question on where the token comes from.
type InvokeAgentInput struct {
	Reads       map[string]manifest.Ref
	Instruction string
}

// InvokeAgentOutput reports the outcome of one invoke-agent call: either
// the agent's result as a new slot revision, or a task handle to check
// with poll-agent-task. It never blocks waiting for a task to finish.
// Result carries the same content as Revision.Ref, in plain text, purely
// so a workflow's final output step can produce text without a separate
// resolve step; every other consumer should read Revision.Ref instead.
type InvokeAgentOutput struct {
	Done     bool
	Revision manifest.Revision
	Result   string
	TaskID   string
}

// PollAgentTaskInput is the Temporal activity input for checking one A2A
// task's status.
type PollAgentTaskInput struct {
	TaskID string
	// ProducedBy and Reason are carried through from the originating
	// invoke-agent call so the revision recorded on completion has the
	// same lineage it would have had if the agent had answered
	// synchronously.
	ProducedBy string
	Reason     string
}

// PollAgentTaskOutput reports the outcome of one status check. It never
// loops or waits — call poll-agent-task again to check again later.
// Result mirrors InvokeAgentOutput.Result — see its doc comment.
type PollAgentTaskOutput struct {
	Done          bool
	Failed        bool
	FailureReason string
	Revision      manifest.Revision
	Result        string
}

// invokeAgentActivity returns an activity function bound to agent that
// resolves its declared input refs, substitutes them into the
// instruction template, sends the result via the Dispatcher, and returns
// immediately, per specs/async-agent-dispatch.md.
func (s *Supervisor) invokeAgentActivity(agent agents.Agent) func(ctx context.Context, in InvokeAgentInput) (InvokeAgentOutput, error) {
	return func(ctx context.Context, in InvokeAgentInput) (InvokeAgentOutput, error) {
		prompt, err := s.resolvePrompt(ctx, in.Reads, in.Instruction)
		if err != nil {
			return InvokeAgentOutput{}, err
		}

		token, err := s.tokens.Token(ctx)
		if err != nil {
			return InvokeAgentOutput{}, err
		}
		result, err := s.dispatcher.Dispatch(ctx, agent.ID, token, prompt)
		if err != nil {
			return InvokeAgentOutput{}, err
		}
		if !result.Done {
			return InvokeAgentOutput{Done: false, TaskID: result.TaskID}, nil
		}

		rev, err := s.storeResult(ctx, agent, "invoke-agent", "initial", result.Result)
		if err != nil {
			return InvokeAgentOutput{}, err
		}
		return InvokeAgentOutput{Done: true, Revision: rev, Result: result.Result}, nil
	}
}

// pollAgentTaskActivity returns an activity function bound to agent that
// checks one A2A task's status once and returns immediately.
func (s *Supervisor) pollAgentTaskActivity(agent agents.Agent) func(ctx context.Context, in PollAgentTaskInput) (PollAgentTaskOutput, error) {
	return func(ctx context.Context, in PollAgentTaskInput) (PollAgentTaskOutput, error) {
		token, err := s.tokens.Token(ctx)
		if err != nil {
			return PollAgentTaskOutput{}, err
		}
		status, err := s.dispatcher.PollTask(ctx, agent.ID, token, in.TaskID)
		if err != nil {
			return PollAgentTaskOutput{}, err
		}
		if !status.Done || status.Failed {
			return PollAgentTaskOutput{
				Done:          status.Done,
				Failed:        status.Failed,
				FailureReason: status.FailureReason,
			}, nil
		}

		rev, err := s.storeResult(ctx, agent, in.ProducedBy, in.Reason, status.Result)
		if err != nil {
			return PollAgentTaskOutput{}, err
		}
		return PollAgentTaskOutput{Done: true, Revision: rev, Result: status.Result}, nil
	}
}

// resolvePrompt fetches every ref in reads from blobstore and substitutes
// its content into instruction wherever "{{slot}}" appears, so the
// activity — not the workflow YAML — is what touches raw content.
func (s *Supervisor) resolvePrompt(ctx context.Context, reads map[string]manifest.Ref, instruction string) (string, error) {
	// Sort keys for deterministic substitution order (irrelevant to the
	// result, but keeps behavior reproducible for tests/debugging).
	slots := make([]string, 0, len(reads))
	for slot := range reads {
		slots = append(slots, slot)
	}
	sort.Strings(slots)

	prompt := instruction
	for _, slot := range slots {
		content, err := s.blobs.Get(ctx, tenant, reads[slot])
		if err != nil {
			return "", fmt.Errorf("resolve slot %q: %w", slot, err)
		}
		prompt = strings.ReplaceAll(prompt, "{{"+slot+"}}", string(content))
	}
	return prompt, nil
}

// storeResult uploads content to blobstore and wraps it as a Revision
// produced by agent, so the workflow can append it to the relevant slot.
func (s *Supervisor) storeResult(ctx context.Context, agent agents.Agent, producedBy, reason, content string) (manifest.Revision, error) {
	ref, err := s.blobs.Put(ctx, tenant, "text/plain", []byte(content))
	if err != nil {
		return manifest.Revision{}, fmt.Errorf("store result for agent %s: %w", agent.ID, err)
	}
	return manifest.Revision{Ref: ref, ProducedBy: producedBy, Reason: reason}, nil
}

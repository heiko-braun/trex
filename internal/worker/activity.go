package worker

import (
	"context"

	"github.com/heiko-braun/trex/internal/agents"
)

// InvokeAgentInput is the Temporal activity input for calling an agent:
// just the workflow's input message text. The A2A call's Bearer token
// is fetched fresh from the Supervisor's TokenSource right before the
// call, not passed in by the workflow — see specs/agent-discovery-and-registration.md's
// open question on where the token comes from.
type InvokeAgentInput struct {
	Input string
}

// InvokeAgentOutput reports the outcome of one invoke-agent call: either
// the agent's direct answer, or a task handle to check with
// poll-agent-task. It never blocks waiting for a task to finish.
type InvokeAgentOutput struct {
	Done   bool
	Result string
	TaskID string
}

// PollAgentTaskInput is the Temporal activity input for checking one A2A
// task's status.
type PollAgentTaskInput struct {
	TaskID string
}

// PollAgentTaskOutput reports the outcome of one status check. It never
// loops or waits — call poll-agent-task again to check again later.
type PollAgentTaskOutput struct {
	Done          bool
	Failed        bool
	FailureReason string
	Result        string
}

// invokeAgentActivity returns an activity function bound to agent that
// sends its input via the Dispatcher and returns immediately, per
// specs/async-agent-dispatch.md.
func (s *Supervisor) invokeAgentActivity(agent agents.Agent) func(ctx context.Context, in InvokeAgentInput) (InvokeAgentOutput, error) {
	return func(ctx context.Context, in InvokeAgentInput) (InvokeAgentOutput, error) {
		token, err := s.tokens.Token(ctx)
		if err != nil {
			return InvokeAgentOutput{}, err
		}
		result, err := s.dispatcher.Dispatch(ctx, agent.ID, token, in.Input)
		if err != nil {
			return InvokeAgentOutput{}, err
		}
		return InvokeAgentOutput{Done: result.Done, Result: result.Result, TaskID: result.TaskID}, nil
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
		return PollAgentTaskOutput{
			Done:          status.Done,
			Failed:        status.Failed,
			FailureReason: status.FailureReason,
			Result:        status.Result,
		}, nil
	}
}

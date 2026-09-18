package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
)

// Dispatcher sends messages to agents via A2A and checks on their task
// status. It holds no credential of its own: every call takes the token
// to use, since that token flows with the Temporal workflow execution
// that triggers the call rather than being held by the server or the
// worker.
//
// Dispatch and PollTask are each a single HTTP call — neither blocks
// waiting for a task to finish. A2A tasks that don't complete
// synchronously are polled by a separate activity call per attempt (see
// specs/async-agent-dispatch.md), so no single call needs a token that
// outlives it.
type Dispatcher struct {
	baseURL string
	http    *http.Client
}

// NewDispatcher builds a Dispatcher against the given control-plane base
// URL.
func NewDispatcher(baseURL string) *Dispatcher {
	return &Dispatcher{baseURL: baseURL, http: http.DefaultClient}
}

// DispatchResult is the outcome of sending a message to an agent: either
// a direct text result (the task already reached a terminal state by
// the time message:send returned) or a task handle to poll later.
type DispatchResult struct {
	Done   bool   // true if Result is the final answer; false if TaskID must be polled
	Result string // set when Done
	TaskID string // set when !Done
}

// TaskStatus is the outcome of checking one A2A task's status.
type TaskStatus struct {
	Done          bool   // true if the task reached a terminal state
	Failed        bool   // true if it terminated unsuccessfully (only meaningful when Done)
	FailureReason string // set when Failed
	Result        string // set when Done && !Failed
}

type a2aTextPart struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

type a2aMessage struct {
	Role      string        `json:"role"`
	Parts     []a2aTextPart `json:"parts"`
	MessageID string        `json:"messageId"`
}

type sendMessageRequest struct {
	Message       a2aMessage        `json:"message"`
	Configuration sendMessageConfig `json:"configuration"`
}

// sendMessageConfig sets returnImmediately so message:send always
// returns a task handle right away instead of blocking server-side
// until the task finishes — confirmed live: without it, a slow agent's
// call can block for 100+ seconds, defeating the whole point of
// splitting dispatch from polling. See specs/async-agent-dispatch.md.
type sendMessageConfig struct {
	ReturnImmediately bool `json:"returnImmediately"`
}

// sendMessageResponse mirrors what the control plane's message:send
// endpoint actually returns: a Task wrapped under a top-level "task" key
// (confirmed live against agents.stage.vibecoding.sixt.cloud — there is
// no bare-"message" variant in practice, and no top-level "kind"
// discriminator).
type sendMessageResponse struct {
	Task a2aTask `json:"task"`
}

// a2aTask mirrors both message:send's wrapped task and GET
// /tasks/{id}'s unwrapped task response — same shape, confirmed live.
type a2aTask struct {
	ID        string        `json:"id"`
	Status    a2aTaskStatus `json:"status"`
	Artifacts []a2aArtifact `json:"artifacts,omitempty"`
}

type a2aArtifact struct {
	Parts []a2aTextPart `json:"parts"`
}

type a2aTaskStatus struct {
	State string `json:"state"` // e.g. TASK_STATE_COMPLETED, TASK_STATE_WORKING, TASK_STATE_FAILED
}

// Dispatch sends input as a new message to agentID, authenticating with
// token, and returns immediately: the task's result if it already
// reached a terminal state, or a task handle otherwise. It never waits
// for a task to finish — call PollTask separately (and repeatedly, each
// call with its own token) to check on it.
func (d *Dispatcher) Dispatch(ctx context.Context, agentID, token, input string) (DispatchResult, error) {
	body, err := json.Marshal(sendMessageRequest{
		Message: a2aMessage{
			Role:      "user",
			Parts:     []a2aTextPart{{Kind: "text", Text: input}},
			MessageID: uuid.NewString(),
		},
		Configuration: sendMessageConfig{ReturnImmediately: true},
	})
	if err != nil {
		return DispatchResult{}, err
	}

	url := fmt.Sprintf("%s/a2a/%s/message:send", d.baseURL, agentID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return DispatchResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.http.Do(req)
	if err != nil {
		return DispatchResult{}, fmt.Errorf("dispatch to agent %s: %w", agentID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return DispatchResult{}, fmt.Errorf("dispatch to agent %s: unexpected status %d", agentID, resp.StatusCode)
	}

	var out sendMessageResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return DispatchResult{}, fmt.Errorf("dispatch to agent %s: decode response: %w", agentID, err)
	}

	return taskToResult(out.Task), nil
}

// PollTask checks agentID's taskID once, authenticating with token, and
// returns its current status. It never loops or waits — call it again
// (with a fresh token, if needed) to check again later.
func (d *Dispatcher) PollTask(ctx context.Context, agentID, token, taskID string) (TaskStatus, error) {
	url := fmt.Sprintf("%s/a2a/%s/tasks/%s", d.baseURL, agentID, taskID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return TaskStatus{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := d.http.Do(req)
	if err != nil {
		return TaskStatus{}, fmt.Errorf("get task %s for agent %s: %w", taskID, agentID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return TaskStatus{}, fmt.Errorf("get task %s for agent %s: unexpected status %d", taskID, agentID, resp.StatusCode)
	}

	var task a2aTask
	if err := json.NewDecoder(resp.Body).Decode(&task); err != nil {
		return TaskStatus{}, fmt.Errorf("get task %s for agent %s: decode response: %w", taskID, agentID, err)
	}

	result := taskToResult(task)
	if result.Done {
		return TaskStatus{Done: true, Result: result.Result}, nil
	}
	if isFailureState(task.Status.State) {
		return TaskStatus{Done: true, Failed: true, FailureReason: task.Status.State}, nil
	}
	return TaskStatus{Done: false}, nil
}

// taskToResult classifies task by its A2A state: TASK_STATE_COMPLETED is
// the only success-terminal state; other terminal states are reported
// as "not done, not a direct result" here — PollTask (which sees them
// through this same helper) further classifies them as Failed.
func taskToResult(task a2aTask) DispatchResult {
	if task.Status.State == "TASK_STATE_COMPLETED" {
		result := ""
		if len(task.Artifacts) > 0 {
			result = joinText(task.Artifacts[0].Parts)
		}
		return DispatchResult{Done: true, Result: result}
	}
	return DispatchResult{Done: false, TaskID: task.ID}
}

// isFailureState reports whether state is one of A2A's unsuccessful
// terminal states.
func isFailureState(state string) bool {
	switch state {
	case "TASK_STATE_FAILED", "TASK_STATE_CANCELED", "TASK_STATE_REJECTED":
		return true
	default:
		return false
	}
}

func joinText(parts []a2aTextPart) string {
	var out string
	for _, p := range parts {
		out += p.Text
	}
	return out
}

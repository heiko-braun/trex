package agents

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Fixture shapes below mirror what the real control plane returns
// (confirmed live against agents.stage.vibecoding.sixt.cloud): a Task
// wrapped under "task" for message:send, unwrapped for GET tasks/{id},
// with the result text under artifacts[0].parts, not status.message.

func TestDispatch_ReturnsDirectResultWhenTaskAlreadyCompleted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(sendMessageResponse{
			Task: a2aTask{
				ID:        "task-123",
				Status:    a2aTaskStatus{State: "TASK_STATE_COMPLETED"},
				Artifacts: []a2aArtifact{{Parts: []a2aTextPart{{Kind: "text", Text: "hello"}}}},
			},
		})
	}))
	defer srv.Close()

	d := NewDispatcher(srv.URL)
	result, err := d.Dispatch(context.Background(), "agent-1", "tok", "hi")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !result.Done || result.Result != "hello" {
		t.Errorf("result = %+v, want Done=true Result=hello", result)
	}
}

func TestDispatch_ReturnsTaskHandleWithoutBlockingWhenStillWorking(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(sendMessageResponse{
			Task: a2aTask{ID: "task-123", Status: a2aTaskStatus{State: "TASK_STATE_WORKING"}},
		})
	}))
	defer srv.Close()

	d := NewDispatcher(srv.URL)
	result, err := d.Dispatch(context.Background(), "agent-1", "tok", "hi")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if result.Done || result.TaskID != "task-123" {
		t.Errorf("result = %+v, want Done=false TaskID=task-123", result)
	}
}

func TestDispatch_PropagatesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	d := NewDispatcher(srv.URL)
	if _, err := d.Dispatch(context.Background(), "agent-1", "tok", "hi"); err == nil {
		t.Fatal("Dispatch: expected error on 401, got nil")
	}
}

func TestPollTask_ReturnsDoneOnCompleted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(a2aTask{
			ID:        "task-123",
			Status:    a2aTaskStatus{State: "TASK_STATE_COMPLETED"},
			Artifacts: []a2aArtifact{{Parts: []a2aTextPart{{Kind: "text", Text: "final answer"}}}},
		})
	}))
	defer srv.Close()

	d := NewDispatcher(srv.URL)
	status, err := d.PollTask(context.Background(), "agent-1", "tok", "task-123")
	if err != nil {
		t.Fatalf("PollTask: %v", err)
	}
	if !status.Done || status.Failed || status.Result != "final answer" {
		t.Errorf("status = %+v, want Done=true Failed=false Result=%q", status, "final answer")
	}
}

func TestPollTask_ReturnsNotDoneWhilePending(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(a2aTask{ID: "task-123", Status: a2aTaskStatus{State: "TASK_STATE_WORKING"}})
	}))
	defer srv.Close()

	d := NewDispatcher(srv.URL)
	status, err := d.PollTask(context.Background(), "agent-1", "tok", "task-123")
	if err != nil {
		t.Fatalf("PollTask: %v", err)
	}
	if status.Done {
		t.Errorf("status = %+v, want Done=false while task is still working", status)
	}
}

func TestPollTask_ReturnsFailedOnTerminalFailureState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(a2aTask{ID: "task-123", Status: a2aTaskStatus{State: "TASK_STATE_FAILED"}})
	}))
	defer srv.Close()

	d := NewDispatcher(srv.URL)
	status, err := d.PollTask(context.Background(), "agent-1", "tok", "task-123")
	if err != nil {
		t.Fatalf("PollTask: %v", err)
	}
	if !status.Done || !status.Failed || status.FailureReason != "TASK_STATE_FAILED" {
		t.Errorf("status = %+v, want Done=true Failed=true FailureReason=TASK_STATE_FAILED", status)
	}
}

// TestPollTask_NeverLoops confirms a single PollTask call issues exactly
// one HTTP request, regardless of the task's reported state — the
// caller (not PollTask) owns any retry/wait loop, per
// specs/async-agent-dispatch.md.
func TestPollTask_NeverLoops(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		json.NewEncoder(w).Encode(a2aTask{ID: "task-123", Status: a2aTaskStatus{State: "TASK_STATE_WORKING"}})
	}))
	defer srv.Close()

	d := NewDispatcher(srv.URL)
	if _, err := d.PollTask(context.Background(), "agent-1", "tok", "task-123"); err != nil {
		t.Fatalf("PollTask: %v", err)
	}
	if calls != 1 {
		t.Errorf("server received %d requests, want exactly 1", calls)
	}
}

package worker

import (
	"context"
	"testing"
	"time"

	"go.temporal.io/sdk/testsuite"

	"github.com/heiko-braun/trex/internal/agents"
	"github.com/heiko-braun/trex/internal/manifest"
)

// slowAsyncDispatcher simulates a real agent that takes many polls to
// finish its task: Dispatch itself returns quickly with a task handle
// (as any real async agent's initial A2A response does), and PollTask
// only reports done on its 3rd call. Since invoke-agent must never block
// waiting for that, this fake's Dispatch never sleeps — only a test that
// mistakenly made invoke-agent wait for completion would time out.
type slowAsyncDispatcher struct {
	pollCount int
}

func (f *slowAsyncDispatcher) Dispatch(_ context.Context, _, _, _ string) (agents.DispatchResult, error) {
	return agents.DispatchResult{Done: false, TaskID: "task-1"}, nil
}

func (f *slowAsyncDispatcher) PollTask(_ context.Context, _, _, _ string) (agents.TaskStatus, error) {
	f.pollCount++
	if f.pollCount < 3 {
		return agents.TaskStatus{Done: false}, nil
	}
	return agents.TaskStatus{Done: true, Result: "eventually done"}, nil
}

func TestInvokeAgentActivity_ReturnsImmediatelyForAsyncTask(t *testing.T) {
	dispatcher := &slowAsyncDispatcher{}
	s := NewSupervisor(devServer.Client(), dispatcher, fakeTokenSource{}, newFakeBlobStore())
	agent := agents.Agent{ID: "agent-1", Name: "test-agent"}

	env := (&testsuite.WorkflowTestSuite{}).NewTestActivityEnvironment()
	env.RegisterActivity(s.invokeAgentActivity(agent))

	start := time.Now()
	val, err := env.ExecuteActivity(s.invokeAgentActivity(agent), InvokeAgentInput{Instruction: "hi"})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("ExecuteActivity: %v", err)
	}
	var out InvokeAgentOutput
	if err := val.Get(&out); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if out.Done || out.TaskID != "task-1" {
		t.Errorf("out = %+v, want Done=false TaskID=task-1", out)
	}
	if elapsed > 5*time.Second {
		t.Errorf("invoke-agent took %v, want near-instant (it must never block on task completion)", elapsed)
	}
}

// recordingDispatcher records the prompt it was given and always
// completes synchronously, for verifying ref resolution and
// substitution.
type recordingDispatcher struct {
	gotPrompt string
}

func (f *recordingDispatcher) Dispatch(_ context.Context, _, _, prompt string) (agents.DispatchResult, error) {
	f.gotPrompt = prompt
	return agents.DispatchResult{Done: true, Result: "agent said: " + prompt}, nil
}

func (f *recordingDispatcher) PollTask(_ context.Context, _, _, _ string) (agents.TaskStatus, error) {
	return agents.TaskStatus{Done: true, Result: "unused"}, nil
}

func TestInvokeAgentActivity_ResolvesRefsAndStoresResult(t *testing.T) {
	dispatcher := &recordingDispatcher{}
	blobs := newFakeBlobStore()
	s := NewSupervisor(devServer.Client(), dispatcher, fakeTokenSource{}, blobs)
	agent := agents.Agent{ID: "agent-1", Name: "test-agent"}

	planRef, err := blobs.Put(context.Background(), tenant, "text/plain", []byte("the plan"))
	if err != nil {
		t.Fatalf("seed plan blob: %v", err)
	}

	env := (&testsuite.WorkflowTestSuite{}).NewTestActivityEnvironment()
	env.RegisterActivity(s.invokeAgentActivity(agent))

	val, err := env.ExecuteActivity(s.invokeAgentActivity(agent), InvokeAgentInput{
		Reads:       map[string]manifest.Ref{"plan": planRef},
		Instruction: "Review this plan: {{plan}}",
	})
	if err != nil {
		t.Fatalf("ExecuteActivity: %v", err)
	}
	if dispatcher.gotPrompt != "Review this plan: the plan" {
		t.Errorf("dispatcher got prompt %q, want ref content substituted into instruction", dispatcher.gotPrompt)
	}

	var out InvokeAgentOutput
	if err := val.Get(&out); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !out.Done {
		t.Fatalf("out.Done = false, want true (dispatcher completes synchronously)")
	}
	if out.Revision.Ref.Digest == "" {
		t.Fatalf("out.Revision.Ref.Digest empty, want a stored result ref")
	}
	if out.Revision.ProducedBy != "invoke-agent" {
		t.Errorf("out.Revision.ProducedBy = %q, want %q", out.Revision.ProducedBy, "invoke-agent")
	}

	stored, err := blobs.Get(context.Background(), tenant, out.Revision.Ref)
	if err != nil {
		t.Fatalf("Get stored result: %v", err)
	}
	if string(stored) != "agent said: Review this plan: the plan" {
		t.Errorf("stored result = %q, want the dispatcher's result content", stored)
	}
}

func TestPollAgentTaskActivity_ChecksOnceAndReturns(t *testing.T) {
	dispatcher := &slowAsyncDispatcher{}
	s := NewSupervisor(devServer.Client(), dispatcher, fakeTokenSource{}, newFakeBlobStore())
	agent := agents.Agent{ID: "agent-1", Name: "test-agent"}

	env := (&testsuite.WorkflowTestSuite{}).NewTestActivityEnvironment()
	env.RegisterActivity(s.pollAgentTaskActivity(agent))

	val, err := env.ExecuteActivity(s.pollAgentTaskActivity(agent), PollAgentTaskInput{TaskID: "task-1"})
	if err != nil {
		t.Fatalf("ExecuteActivity: %v", err)
	}
	var out PollAgentTaskOutput
	if err := val.Get(&out); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if out.Done {
		t.Errorf("out = %+v, want Done=false on first poll (fake reports not-done until 3rd call)", out)
	}
	if dispatcher.pollCount != 1 {
		t.Errorf("PollTask called %d times, want exactly 1 per activity invocation", dispatcher.pollCount)
	}
}

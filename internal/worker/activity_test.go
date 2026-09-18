package worker

import (
	"context"
	"testing"
	"time"

	"go.temporal.io/sdk/testsuite"

	"github.com/heiko-braun/trex/internal/agents"
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
	s := NewSupervisor(devServer.Client(), dispatcher, fakeTokenSource{})
	agent := agents.Agent{ID: "agent-1", Name: "test-agent"}

	env := (&testsuite.WorkflowTestSuite{}).NewTestActivityEnvironment()
	env.RegisterActivity(s.invokeAgentActivity(agent))

	start := time.Now()
	val, err := env.ExecuteActivity(s.invokeAgentActivity(agent), InvokeAgentInput{Input: "hi"})
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

func TestPollAgentTaskActivity_ChecksOnceAndReturns(t *testing.T) {
	dispatcher := &slowAsyncDispatcher{}
	s := NewSupervisor(devServer.Client(), dispatcher, fakeTokenSource{})
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

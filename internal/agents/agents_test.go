package agents

import "testing"

func TestAgent_Slug(t *testing.T) {
	a := Agent{ID: "decorous-word-8592"}
	if got := a.Slug(); got != "decorous-word-8592" {
		t.Errorf("Slug() = %q, want %q", got, "decorous-word-8592")
	}
}

func TestAgent_TaskQueue(t *testing.T) {
	a := Agent{ID: "decorous-word-8592"}
	if got := a.TaskQueue(); got != "agent-decorous-word-8592" {
		t.Errorf("TaskQueue() = %q, want %q", got, "agent-decorous-word-8592")
	}
}

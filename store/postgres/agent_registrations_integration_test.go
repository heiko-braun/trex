//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/heiko-braun/trex/store"
)

func newTestRegistrationStore(t *testing.T) *AgentRegistrationStore {
	t.Helper()
	connStr := testConnStr(t)

	if err := RunMigrations(connStr); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	pool, err := pgxpool.New(context.Background(), connStr)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	return NewAgentRegistrationStore(pool)
}

func uniqueAgentID(t *testing.T) string {
	t.Helper()
	return "agent-" + t.Name()
}

func TestAgentRegistrationStore_RegisterAndList(t *testing.T) {
	s := newTestRegistrationStore(t)
	ctx := context.Background()
	agentID := uniqueAgentID(t)

	reg := &store.AgentRegistration{
		AgentID:   agentID,
		Name:      "test-agent",
		TaskQueue: "agent-" + agentID,
	}

	created, err := s.Register(ctx, reg)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if created.AgentID != agentID {
		t.Errorf("AgentID = %q, want %q", created.AgentID, agentID)
	}

	all, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var found bool
	for _, r := range all {
		if r.AgentID == agentID {
			found = true
		}
	}
	if !found {
		t.Errorf("registered agent %q not found in List()", agentID)
	}
}

func TestAgentRegistrationStore_RegisterIsIdempotent(t *testing.T) {
	s := newTestRegistrationStore(t)
	ctx := context.Background()
	agentID := uniqueAgentID(t)

	reg := &store.AgentRegistration{AgentID: agentID, Name: "test-agent", TaskQueue: "agent-" + agentID}

	first, err := s.Register(ctx, reg)
	if err != nil {
		t.Fatalf("first Register: %v", err)
	}
	second, err := s.Register(ctx, reg)
	if err != nil {
		t.Fatalf("second Register: %v", err)
	}
	if first.RegisteredAt != second.RegisteredAt {
		t.Errorf("expected re-register to return the SAME row, got different RegisteredAt: %v vs %v",
			first.RegisteredAt, second.RegisteredAt)
	}
}

func TestAgentRegistrationStore_Unregister(t *testing.T) {
	s := newTestRegistrationStore(t)
	ctx := context.Background()
	agentID := uniqueAgentID(t)

	if _, err := s.Register(ctx, &store.AgentRegistration{AgentID: agentID, Name: "test-agent", TaskQueue: "agent-" + agentID}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := s.Unregister(ctx, agentID); err != nil {
		t.Fatalf("Unregister: %v", err)
	}

	all, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, r := range all {
		if r.AgentID == agentID {
			t.Errorf("agent %q still present after Unregister", agentID)
		}
	}
}

func TestAgentRegistrationStore_UnregisterUnknownIsNoop(t *testing.T) {
	s := newTestRegistrationStore(t)
	ctx := context.Background()

	if err := s.Unregister(ctx, uniqueAgentID(t)); err != nil {
		t.Fatalf("Unregister: %v", err)
	}
}

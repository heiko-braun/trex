//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/heiko-braun/trex/store"
)

func testConnStr(t *testing.T) string {
	t.Helper()
	s := os.Getenv("TEST_DATABASE_URL")
	if s == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping Postgres integration test")
	}
	return s
}

func newTestStore(t *testing.T) *DefinitionStore {
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

	// isolate each test with a unique tenant so parallel/repeated runs
	// against the same database don't collide.
	return NewDefinitionStore(pool)
}

func uniqueTenant(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("tenant-%s", t.Name())
}

func TestDefinitionStore_CreateAndGetCurrent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	tenant := uniqueTenant(t)

	def := &store.Definition{
		Tenant:  tenant,
		Name:    "my-workflow",
		BuildID: "abc123",
		YAML:    "document: {}",
		Status:  store.StatusPending,
	}

	created, err := s.Create(ctx, def)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.BuildID != def.BuildID {
		t.Errorf("BuildID = %q, want %q", created.BuildID, def.BuildID)
	}

	got, err := s.GetCurrent(ctx, tenant, "my-workflow")
	if err != nil {
		t.Fatalf("GetCurrent: %v", err)
	}
	if got.YAML != def.YAML {
		t.Errorf("YAML = %q, want %q", got.YAML, def.YAML)
	}
	if got.Status != store.StatusPending {
		t.Errorf("Status = %q, want %q", got.Status, store.StatusPending)
	}
}

func TestDefinitionStore_CreateIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	tenant := uniqueTenant(t)

	def := &store.Definition{
		Tenant:  tenant,
		Name:    "my-workflow",
		BuildID: "same-build-id",
		YAML:    "document: {}",
		Status:  store.StatusPending,
	}

	first, err := s.Create(ctx, def)
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}
	second, err := s.Create(ctx, def)
	if err != nil {
		t.Fatalf("second Create: %v", err)
	}
	if first.CreatedAt != second.CreatedAt {
		t.Errorf("expected re-publish to return the SAME row, got different CreatedAt: %v vs %v",
			first.CreatedAt, second.CreatedAt)
	}

	// still exactly one current definition, not a duplicate.
	got, err := s.GetCurrent(ctx, tenant, "my-workflow")
	if err != nil {
		t.Fatalf("GetCurrent: %v", err)
	}
	if got.BuildID != def.BuildID {
		t.Errorf("BuildID = %q, want %q", got.BuildID, def.BuildID)
	}
}

func TestDefinitionStore_GetCurrentReturnsLatest(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	tenant := uniqueTenant(t)

	first := &store.Definition{Tenant: tenant, Name: "my-workflow", BuildID: "v1", YAML: "v1", Status: store.StatusPending}
	second := &store.Definition{Tenant: tenant, Name: "my-workflow", BuildID: "v2", YAML: "v2", Status: store.StatusPending}

	if _, err := s.Create(ctx, first); err != nil {
		t.Fatalf("Create first: %v", err)
	}
	if _, err := s.Create(ctx, second); err != nil {
		t.Fatalf("Create second: %v", err)
	}

	got, err := s.GetCurrent(ctx, tenant, "my-workflow")
	if err != nil {
		t.Fatalf("GetCurrent: %v", err)
	}
	if got.BuildID != "v2" {
		t.Errorf("BuildID = %q, want %q (the more recently created)", got.BuildID, "v2")
	}
}

func TestDefinitionStore_GetCurrentNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.GetCurrent(ctx, uniqueTenant(t), "nonexistent")
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("expected store.ErrNotFound, got: %v", err)
	}
}

func TestDefinitionStore_ListReturnsNewestFirst(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	tenant := uniqueTenant(t)

	first := &store.Definition{Tenant: tenant, Name: "my-workflow", BuildID: "v1", YAML: "v1", Status: store.StatusPending}
	second := &store.Definition{Tenant: tenant, Name: "my-workflow", BuildID: "v2", YAML: "v2", Status: store.StatusPending}
	other := &store.Definition{Tenant: tenant, Name: "other-workflow", BuildID: "v1", YAML: "v1", Status: store.StatusPending}

	if _, err := s.Create(ctx, first); err != nil {
		t.Fatalf("Create first: %v", err)
	}
	if _, err := s.Create(ctx, second); err != nil {
		t.Fatalf("Create second: %v", err)
	}
	if _, err := s.Create(ctx, other); err != nil {
		t.Fatalf("Create other: %v", err)
	}

	all, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	// The table isn't isolated per test, so filter down to rows this test
	// created and check their relative order within that subset.
	var got []*store.Definition
	for _, d := range all {
		if d.Tenant == tenant {
			got = append(got, d)
		}
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 definitions for tenant %q, got %d", tenant, len(got))
	}
	if got[0].BuildID != "v1" || got[0].Name != "other-workflow" {
		t.Errorf("got[0] = %+v, want the most recently created (other-workflow/v1)", got[0])
	}
	if got[1].BuildID != "v2" || got[1].Name != "my-workflow" {
		t.Errorf("got[1] = %+v, want my-workflow/v2", got[1])
	}
	if got[2].BuildID != "v1" || got[2].Name != "my-workflow" {
		t.Errorf("got[2] = %+v, want my-workflow/v1", got[2])
	}
}

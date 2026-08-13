package controller

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/amirarzideh/MikroTunnel/internal/domain"
	"github.com/amirarzideh/MikroTunnel/internal/store"
)

func TestReconcileCompletesCreateAndDeleteOperations(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, err := store.Open(t.TempDir() + "/mikrotunnel.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	now := time.Now().UTC()
	tunnel := domain.Tunnel{ID: "tunnel-1", Name: "gre-test", Type: domain.TunnelGRE, Local: "198.51.100.10", Remote: "203.0.113.20", Address: "10.10.0.1/30", MTU: 1476, TTL: 255, DesiredState: domain.DesiredEnabled, ActualState: domain.ActualPending, CreatedAt: now, UpdatedAt: now}
	if err := db.CreateTunnel(ctx, tunnel); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateOperation(ctx, domain.Operation{ID: "op-create", Action: "create_tunnel", ResourceType: "tunnel", ResourceID: tunnel.ID, Status: domain.OperationQueued, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	provider := &fakeProvider{}
	reconciler := New(db, []domain.TunnelProvider{provider}, slog.Default())
	if err := reconciler.ReconcileOne(ctx, tunnel); err != nil {
		t.Fatal(err)
	}
	operations, err := db.ListOperations(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if operations[0].Status != domain.OperationSuccess {
		t.Fatalf("expected successful create operation, got %#v", operations[0])
	}

	tunnel, err = db.GetTunnel(ctx, tunnel.ID)
	if err != nil {
		t.Fatal(err)
	}
	tunnel.DesiredState = domain.DesiredDeleted
	if err := db.UpdateTunnel(ctx, tunnel); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateOperation(ctx, domain.Operation{ID: "op-delete", Action: "delete_tunnel", ResourceType: "tunnel", ResourceID: tunnel.ID, Status: domain.OperationQueued, CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if err := reconciler.ReconcileOne(ctx, tunnel); err != nil {
		t.Fatal(err)
	}
	if !provider.removed {
		t.Fatal("expected provider removal")
	}
	if _, err := db.GetTunnel(ctx, tunnel.ID); err != store.ErrNotFound {
		t.Fatalf("expected deleted tunnel, got %v", err)
	}
	operations, err = db.ListOperations(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if operations[0].Action != "delete_tunnel" || operations[0].Status != domain.OperationSuccess {
		t.Fatalf("expected successful delete operation, got %#v", operations[0])
	}
}

type fakeProvider struct{ removed bool }

func (p *fakeProvider) Type() domain.TunnelType                        { return domain.TunnelGRE }
func (p *fakeProvider) Validate(domain.TunnelInput) error              { return nil }
func (p *fakeProvider) Reconcile(context.Context, domain.Tunnel) error { return nil }
func (p *fakeProvider) Observe(context.Context, domain.Tunnel) (domain.ActualState, string, error) {
	return domain.ActualUp, "", nil
}
func (p *fakeProvider) Remove(context.Context, domain.Tunnel) error {
	p.removed = true
	return nil
}

package controller

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/amirarzideh/MikroTunnel/internal/domain"
)

type Reconciler struct {
	store     domain.TunnelStore
	providers map[domain.TunnelType]domain.TunnelProvider
	logger    *slog.Logger
}

func New(s domain.TunnelStore, providers []domain.TunnelProvider, logger *slog.Logger) *Reconciler {
	index := make(map[domain.TunnelType]domain.TunnelProvider, len(providers))
	for _, p := range providers {
		index[p.Type()] = p
	}
	return &Reconciler{store: s, providers: index, logger: logger}
}
func (r *Reconciler) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	if err := r.store.RequeueInterruptedOperations(ctx); err != nil {
		r.logger.Error("requeue interrupted operations", "error", err)
	}
	r.ReconcileAll(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.ReconcileAll(ctx)
		}
	}
}
func (r *Reconciler) ReconcileAll(ctx context.Context) {
	tunnels, err := r.store.ListTunnels(ctx)
	if err != nil {
		r.logger.Error("list desired tunnels", "error", err)
		return
	}
	for _, t := range tunnels {
		if err := r.ReconcileOne(ctx, t); err != nil {
			r.logger.Error("reconcile tunnel", "tunnel", t.ID, "error", err)
		}
	}
}
func (r *Reconciler) ReconcileOne(ctx context.Context, t domain.Tunnel) error {
	p, ok := r.providers[t.Type]
	if !ok {
		return r.fail(t, fmt.Sprintf("provider %q is not installed", t.Type))
	}
	if err := r.store.MarkOperationsRunning(ctx, t.ID); err != nil {
		return err
	}
	if t.DesiredState == domain.DesiredDeleted {
		if err := p.Remove(ctx, t); err != nil {
			return r.fail(t, err.Error())
		}
		if err := r.store.DeleteTunnel(ctx, t.ID); err != nil {
			return err
		}
		return r.store.CompleteOperations(ctx, t.ID, domain.OperationSuccess, "tunnel removed")
	}
	if err := p.Reconcile(ctx, t); err != nil {
		return r.fail(t, err.Error())
	}
	state, message, err := p.Observe(ctx, t)
	if err != nil {
		return r.fail(t, err.Error())
	}
	if err := r.mark(t, state, message); err != nil {
		return err
	}
	if state == domain.ActualError || state == domain.ActualUnknown || state == domain.ActualMissing {
		return r.fail(t, message)
	}
	return r.store.CompleteOperations(ctx, t.ID, domain.OperationSuccess, "desired state applied")
}
func (r *Reconciler) mark(t domain.Tunnel, state domain.ActualState, message string) error {
	t.ActualState = state
	t.LastError = message
	t.UpdatedAt = time.Now()
	return r.store.UpdateTunnel(context.Background(), t)
}

func (r *Reconciler) fail(t domain.Tunnel, message string) error {
	if err := r.mark(t, domain.ActualError, message); err != nil {
		return err
	}
	return r.store.CompleteOperations(context.Background(), t.ID, domain.OperationFailed, message)
}

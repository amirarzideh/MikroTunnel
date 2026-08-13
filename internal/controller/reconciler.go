package controller

import (
	"context"
	"fmt"
	"github.com/amirarzideh/MikroTunnel/internal/domain"
	"log/slog"
	"time"
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
		return r.mark(t, domain.ActualError, fmt.Sprintf("provider %q is not installed", t.Type))
	}
	if err := p.Reconcile(ctx, t); err != nil {
		return r.mark(t, domain.ActualError, err.Error())
	}
	state, message, err := p.Observe(ctx, t)
	if err != nil {
		return r.mark(t, domain.ActualError, err.Error())
	}
	return r.mark(t, state, message)
}
func (r *Reconciler) mark(t domain.Tunnel, state domain.ActualState, message string) error {
	t.ActualState = state
	t.LastError = message
	t.UpdatedAt = time.Now()
	return r.store.UpdateTunnel(context.Background(), t)
}

package gre

import (
	"context"
	"fmt"
	"net/netip"
	"regexp"

	"github.com/amirarzideh/MikroTunnel/internal/domain"
)

var namePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,14}$`)

// Provider owns only interfaces explicitly marked by MikroTunnel.
type Provider struct{}

func (Provider) Type() domain.TunnelType { return domain.TunnelGRE }
func (Provider) Validate(in domain.TunnelInput) error {
	if !namePattern.MatchString(in.Name) {
		return fmt.Errorf("name must be 1-15 characters: letters, digits, _ or -")
	}
	local, err := netip.ParseAddr(in.Local)
	if err != nil {
		return fmt.Errorf("invalid local endpoint")
	}
	remote, err := netip.ParseAddr(in.Remote)
	if err != nil || remote == local {
		return fmt.Errorf("invalid remote endpoint")
	}
	if _, err := netip.ParsePrefix(in.Address); err != nil {
		return fmt.Errorf("invalid tunnel address")
	}
	if in.MTU != 0 && (in.MTU < 576 || in.MTU > 9000) {
		return fmt.Errorf("mtu must be between 576 and 9000")
	}
	if in.TTL != 0 && (in.TTL < 1 || in.TTL > 255) {
		return fmt.Errorf("ttl must be between 1 and 255")
	}
	return nil
}
func (Provider) Observe(ctx context.Context, tunnel domain.Tunnel) (domain.ActualState, string, error) {
	return observePlatform(ctx, tunnel)
}
func (Provider) Reconcile(ctx context.Context, tunnel domain.Tunnel) error {
	return reconcilePlatform(ctx, tunnel)
}
func (Provider) Remove(ctx context.Context, tunnel domain.Tunnel) error {
	return removePlatform(ctx, tunnel)
}

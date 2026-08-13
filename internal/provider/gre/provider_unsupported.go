//go:build !linux

package gre

import (
	"context"
	"errors"

	"github.com/amirarzideh/MikroTunnel/internal/domain"
)

func observePlatform(context.Context, domain.Tunnel) (domain.ActualState, string, error) {
	return domain.ActualPending, "GRE management requires Linux", nil
}

func reconcilePlatform(context.Context, domain.Tunnel) error {
	return errors.New("GRE management requires Linux")
}

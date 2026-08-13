//go:build linux

package gre

import (
	"context"
	"errors"
	"fmt"
	"net"
	"syscall"

	"github.com/vishvananda/netlink"

	"github.com/amirarzideh/MikroTunnel/internal/domain"
)

func ownershipMarker(tunnel domain.Tunnel) string { return "mikrotunnel:" + tunnel.ID }

func observePlatform(_ context.Context, tunnel domain.Tunnel) (domain.ActualState, string, error) {
	link, err := netlink.LinkByName(tunnel.Name)
	if isLinkNotFound(err) {
		return domain.ActualMissing, "managed GRE interface is absent", nil
	}
	if err != nil {
		return domain.ActualUnknown, "", fmt.Errorf("read GRE interface: %w", err)
	}
	if link.Attrs().Alias != ownershipMarker(tunnel) {
		return domain.ActualError, "an interface with this name is not owned by MikroTunnel", nil
	}
	if link.Attrs().Flags&net.FlagUp != 0 {
		return domain.ActualUp, "", nil
	}
	return domain.ActualDown, "interface is administratively down", nil
}

func reconcilePlatform(_ context.Context, tunnel domain.Tunnel) error {
	link, err := netlink.LinkByName(tunnel.Name)
	if isLinkNotFound(err) {
		if tunnel.DesiredState == domain.DesiredDisabled {
			return nil
		}
		link, err = create(tunnel)
		if err != nil {
			return err
		}
	} else if err != nil {
		return fmt.Errorf("find GRE interface: %w", err)
	} else if link.Attrs().Alias != ownershipMarker(tunnel) {
		return fmt.Errorf("refusing to manage %q because it is not owned by MikroTunnel", tunnel.Name)
	} else if requiresRecreate(link, tunnel) {
		if err := netlink.LinkDel(link); err != nil {
			return fmt.Errorf("replace changed GRE interface: %w", err)
		}
		link, err = create(tunnel)
		if err != nil {
			return err
		}
	}
	if err := netlink.LinkSetMTU(link, tunnel.MTU); err != nil {
		return fmt.Errorf("set MTU: %w", err)
	}
	if err := ensureAddress(link, tunnel.Address); err != nil {
		return err
	}
	if tunnel.DesiredState == domain.DesiredDisabled {
		if err := netlink.LinkSetDown(link); err != nil {
			return fmt.Errorf("disable GRE interface: %w", err)
		}
		return nil
	}
	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("enable GRE interface: %w", err)
	}
	return nil
}

func removePlatform(_ context.Context, tunnel domain.Tunnel) error {
	link, err := netlink.LinkByName(tunnel.Name)
	if isLinkNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("find GRE interface for deletion: %w", err)
	}
	if link.Attrs().Alias != ownershipMarker(tunnel) {
		return fmt.Errorf("refusing to delete %q because it is not owned by MikroTunnel", tunnel.Name)
	}
	if err := netlink.LinkDel(link); err != nil {
		return fmt.Errorf("delete GRE interface: %w", err)
	}
	return nil
}

func create(tunnel domain.Tunnel) (netlink.Link, error) {
	link := &netlink.Gretun{LinkAttrs: netlink.LinkAttrs{Name: tunnel.Name, MTU: tunnel.MTU, Alias: ownershipMarker(tunnel)}, Local: net.ParseIP(tunnel.Local), Remote: net.ParseIP(tunnel.Remote), Ttl: uint8(tunnel.TTL)}
	if err := netlink.LinkAdd(link); err != nil && !errors.Is(err, syscall.EEXIST) {
		return nil, fmt.Errorf("create GRE interface: %w", err)
	}
	created, err := netlink.LinkByName(tunnel.Name)
	if err != nil {
		return nil, fmt.Errorf("load created GRE interface: %w", err)
	}
	if created.Attrs().Alias != ownershipMarker(tunnel) {
		return nil, fmt.Errorf("refusing to manage %q because it is not owned by MikroTunnel", tunnel.Name)
	}
	return created, nil
}

func ensureAddress(link netlink.Link, cidr string) error {
	addr, err := netlink.ParseAddr(cidr)
	if err != nil {
		return fmt.Errorf("parse interface address: %w", err)
	}
	addresses, err := netlink.AddrList(link, netlink.FAMILY_V4)
	if err != nil {
		return fmt.Errorf("list interface addresses: %w", err)
	}
	for _, current := range addresses {
		if current.String() == addr.String() {
			continue
		}
		if err := netlink.AddrDel(link, &current); err != nil {
			return fmt.Errorf("remove drifted interface address: %w", err)
		}
	}
	if err := netlink.AddrReplace(link, addr); err != nil {
		return fmt.Errorf("assign interface address: %w", err)
	}
	return nil
}

func requiresRecreate(link netlink.Link, tunnel domain.Tunnel) bool {
	greLink, ok := link.(*netlink.Gretun)
	if !ok {
		return true
	}
	return !greLink.Local.Equal(net.ParseIP(tunnel.Local)) || !greLink.Remote.Equal(net.ParseIP(tunnel.Remote)) || greLink.Ttl != uint8(tunnel.TTL)
}

func isLinkNotFound(err error) bool {
	var target netlink.LinkNotFoundError
	return errors.As(err, &target)
}

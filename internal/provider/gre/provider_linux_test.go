//go:build linux

package gre

import (
	"errors"
	"fmt"
	"testing"

	"github.com/vishvananda/netlink"
)

func TestIsLinkNotFound(t *testing.T) {
	if !isLinkNotFound(netlink.LinkNotFoundError{}) {
		t.Fatal("LinkNotFoundError was not recognized")
	}
	if !isLinkNotFound(fmt.Errorf("wrapped: %w", netlink.LinkNotFoundError{})) {
		t.Fatal("wrapped LinkNotFoundError was not recognized")
	}
	if isLinkNotFound(errors.New("Link not found")) {
		t.Fatal("plain text error must not be treated as a LinkNotFoundError")
	}
}

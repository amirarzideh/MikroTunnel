package gre

import (
	"github.com/amirarzideh/MikroTunnel/internal/domain"
	"testing"
)

func TestValidate(t *testing.T) {
	p := Provider{}
	valid := domain.TunnelInput{Name: "gre-main", Type: domain.TunnelGRE, Local: "198.51.100.10", Remote: "203.0.113.10", Address: "10.10.0.1/30", MTU: 1476, TTL: 255}
	if err := p.Validate(valid); err != nil {
		t.Fatalf("valid input rejected: %v", err)
	}
	valid.Remote = valid.Local
	if err := p.Validate(valid); err == nil {
		t.Fatal("same endpoints were accepted")
	}
}

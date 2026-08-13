package domain

import (
	"context"
	"time"
)

type TunnelType string

const (
	TunnelGRE TunnelType = "gre"
)

type DesiredState string

const (
	DesiredEnabled  DesiredState = "enabled"
	DesiredDisabled DesiredState = "disabled"
	DesiredDeleted  DesiredState = "deleted"
)

type ActualState string

const (
	ActualUnknown ActualState = "unknown"
	ActualPending ActualState = "pending"
	ActualUp      ActualState = "up"
	ActualDown    ActualState = "down"
	ActualMissing ActualState = "missing"
	ActualError   ActualState = "error"
)

type Tunnel struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Type         TunnelType   `json:"type"`
	Local        string       `json:"local_endpoint"`
	Remote       string       `json:"remote_endpoint"`
	Address      string       `json:"address"`
	MTU          int          `json:"mtu"`
	TTL          int          `json:"ttl"`
	Description  string       `json:"description,omitempty"`
	DesiredState DesiredState `json:"desired_state"`
	ActualState  ActualState  `json:"actual_state"`
	LastError    string       `json:"last_error,omitempty"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
}

type TunnelInput struct {
	Name        string     `json:"name"`
	Type        TunnelType `json:"type"`
	Local       string     `json:"local_endpoint"`
	Remote      string     `json:"remote_endpoint"`
	Address     string     `json:"address"`
	MTU         int        `json:"mtu"`
	TTL         int        `json:"ttl"`
	Description string     `json:"description"`
}

type TunnelProvider interface {
	Type() TunnelType
	Validate(TunnelInput) error
	Observe(context.Context, Tunnel) (ActualState, string, error)
	Reconcile(context.Context, Tunnel) error
	Remove(context.Context, Tunnel) error
}

package domain

import "context"

type TunnelStore interface {
	CreateTunnel(context.Context, Tunnel) error
	UpdateTunnel(context.Context, Tunnel) error
	DeleteTunnel(context.Context, string) error
	GetTunnel(context.Context, string) (Tunnel, error)
	ListTunnels(context.Context) ([]Tunnel, error)
	CreateOperation(context.Context, Operation) error
	ListOperations(context.Context, int) ([]Operation, error)
	MarkOperationsRunning(context.Context, string) error
	CompleteOperations(context.Context, string, OperationStatus, string) error
	RequeueInterruptedOperations(context.Context) error
	HasAPIKeys(context.Context) (bool, error)
	CreateAPIKey(context.Context, APIKey) error
	FindAPIKey(context.Context, string) (APIKey, error)
}

type APIKey struct {
	ID        string
	Prefix    string
	Hash      string
	CreatedAt int64
	RevokedAt *int64
}

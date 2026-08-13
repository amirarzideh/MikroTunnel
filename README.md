# MikroTunnel

MikroTunnel is a self-hosted Ubuntu network agent for declarative tunnel management.

It is built to run independently first. MikroPanel will later connect to its versioned API using a scoped API key; it will not receive shell access to the server.

## Phase 1 status

The initial foundation provides:

- versioned `/api/v1` HTTP API
- API-key authentication with hashed keys at rest
- SQLite state store
- tunnel desired-state CRUD and operation audit records
- health and system discovery endpoints
- Linux GRE provider with explicit ownership checks, plus provider/controller interfaces for future tunnel types
- systemd and GitHub Release installer templates

On Ubuntu, the GRE provider creates, configures, enables and deletes only interfaces marked as owned by the matching MikroTunnel tunnel ID. A conflicting or manually created interface is never adopted automatically. Deletes are durable operations: the API queues deletion, then the controller removes the owned interface and marks the operation successful only after that succeeds. A tunnel record is not presented as applied until reconciliation reports it.

## Core rule

The database records desired state. The controller is the only component allowed to turn that state into Linux networking changes. The API never accepts arbitrary shell commands, nftables rules, or raw `ip` commands.

## Local development

Requires Go 1.24 or newer.

```bash
go run ./cmd/mikrotunnel serve --config ./configs/development.yaml
```

The first startup creates the database and prints a one-time bootstrap API key if no API key exists.

## API outline

```text
GET    /healthz
GET    /api/v1/system
GET    /api/v1/tunnels
POST   /api/v1/tunnels
GET    /api/v1/tunnels/{id}
PUT    /api/v1/tunnels/{id}
DELETE /api/v1/tunnels/{id}
POST   /api/v1/tunnels/{id}/enable
POST   /api/v1/tunnels/{id}/disable
GET    /api/v1/operations
```

Every `/api/v1` request requires `Authorization: Bearer mt_...`.

## Installer and releases

The installer is designed for GitHub Releases. After a release is published, the deployment command is:

```bash
curl -fsSL https://raw.githubusercontent.com/amirarzideh/MikroTunnel/main/scripts/install.sh | sudo bash
```

## Repository layout

```text
cmd/                 binary entry points
internal/api/        HTTP transport and authentication
internal/domain/     tunnel, operation and provider contracts
internal/store/      SQLite persistence
internal/controller/ desired-vs-actual reconciliation
internal/system/     safe Linux/system discovery adapters
deploy/              systemd assets
scripts/             install and release helpers
```

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
- embedded local dashboard for monitoring and GRE tunnel creation
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

## Service control

The installer provides both `mikrotunnel` and the short alias `mikrotun`.

```bash
sudo mikrotun service status
sudo mikrotun service restart
sudo mikrotun service stop
sudo mikrotun service start
sudo mikrotun service disable
sudo mikrotun service enable
sudo mikrotun uninstall --yes          # keeps configuration and tunnel state
sudo mikrotun uninstall --yes --purge  # permanently removes configuration and state
```

The reconciliation controller repairs missing, down, or drifted owned GRE interfaces. Failed repair attempts are retried automatically with capped exponential backoff (5 seconds to 5 minutes); a clean observation resets the retry counter. `systemd` also restarts the agent if its process exits.

## Secure remote dashboard

The agent always remains bound to loopback. On first installation, secure remote access is a required interactive step: it detects the public IP as the default hostname, requests only an email address, then obtains the certificate itself. Use a domain for a conventional automatically renewed Caddy certificate. Keeping the public-IP default uses a short-lived Let's Encrypt IP certificate and installs an automatic Certbot renewal timer.

```bash
sudo mikrotun setup https
```

The wizard publishes only Caddy on ports 80/443 and leaves MikroTunnel at `127.0.0.1:8787`. For non-interactive automation only, set `MIKROTUNNEL_SKIP_HTTPS=1`; that deliberately leaves remote access disabled.

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

## Dashboard security

The dashboard is served by the agent at `/`. It asks for an API key at connection time and retains it only in the current browser tab. It is not a replacement for transport security: keep the default loopback listener for local use, or put a TLS-enabled reverse proxy and access control in front of a remotely exposed agent.

## Installer and releases

The installer always downloads the latest published GitHub Release. The deployment command is:

```bash
curl -fsSL https://raw.githubusercontent.com/amirarzideh/MikroTunnel/main/scripts/install.sh | sudo bash
```

For a private repository, create a fine-grained GitHub token with read-only **Contents** access to this repository, then use:

```bash
export GITHUB_TOKEN='github_pat_...'
curl -fsSL -H "Authorization: Bearer ${GITHUB_TOKEN}" https://raw.githubusercontent.com/amirarzideh/MikroTunnel/main/scripts/install.sh | sudo -E bash
unset GITHUB_TOKEN
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

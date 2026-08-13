# MikroTunnel delivery roadmap

This repository is deliberately built as a standalone, self-hosted control plane. MikroPanel becomes a client of its API only after the agent has passed Ubuntu integration tests.

## Milestone 1 — agent foundation (implemented)

- Versioned HTTP API with API-key authentication.
- SQLite desired state, audit operations and system discovery.
- Provider/controller contracts, GRE validation and reconciliation seam.
- systemd unit, release workflow and a release-based Ubuntu installer.

**Boundary:** desired state is durable and separate from observed state. An API response never claims a tunnel is usable until the provider reports it.

## Milestone 2 — Ubuntu GRE provider (in progress)

- Add, configure, enable and disable only explicitly MikroTunnel-owned Linux GRE interfaces.
- Queue live-interface deletion durably, then remove only the owned interface.
- Reconcile address, MTU, TTL and link state idempotently.
- Detect manual changes and surface a precise drift/error state.
- Test with disposable Ubuntu virtual machines and namespace fixtures before production use.

## Milestone 3 — operational API (in progress)

- Durable operation queue: queued, running, success or failed (implemented).
- Idempotency keys, request correlation IDs and pagination.
- API-key rotation/revocation, TLS/reverse-proxy guidance and backup/restore.

## Milestone 4 — standalone dashboard

- A compact modern local dashboard served by the agent.
- Live tunnel status, topology and operation history.
- Visual depth used only for hierarchy and state, never decoration that obscures controls.

## Milestone 5 — MikroPanel connector

- Add agent by URL and API key, verify its identity, then store an encrypted credential.
- Map MikroPanel locations/routers to a selected MikroTunnel agent.
- Keep tunnel provisioning asynchronous: MikroPanel shows the remote operation state instead of assuming success.

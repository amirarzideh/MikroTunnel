# Architecture

## Trust boundary

```text
MikroPanel / standalone dashboard
              |
              | authenticated REST request
              v
          MikroTunnel API
              |
              v
        desired-state store
              |
              v
       reconciliation controller
              |
              v
   validated Linux provider operations
```

The HTTP layer validates data and writes desired state. It does not run arbitrary commands. Providers receive typed, validated configuration and own the Linux operation details.

## Extension points

- `domain.TunnelProvider`: protocol-specific validation, observation and reconciliation.
- `domain.TunnelStore`: persistence boundary; SQLite is the initial implementation.
- `controller.Reconciler`: schedules repair/application attempts and records their result.
- `system.Inspector`: read-only host discovery; mutating system work is provider-owned.

## State vocabulary

`desired_state` is `enabled`, `disabled`, or `deleted`. `actual_state` is a provider observation: `unknown`, `pending`, `up`, `down`, `error`, or `missing`. A tunnel stores a failure count and the next eligible retry time; these are operational state, never user configuration.

Every state transition creates an immutable operation record. The controller advances the record through `queued`, `running`, `success`, or `failed`. On startup, interrupted `running` operations are safely returned to `queued`. This lets a future dashboard and MikroPanel report pending or failed networking work accurately rather than claiming that a request immediately succeeded on Linux.

## Self-healing contract

The controller owns convergence. For an enabled GRE tunnel it continuously verifies the owned Linux interface and restores the desired MTU, address and link state. A change to GRE endpoints or TTL is handled by deleting and recreating only the interface carrying that tunnel's ownership marker. It never adopts, alters or deletes an unmarked interface with a conflicting name.

Failures use capped exponential backoff: 5 seconds, 10 seconds, 20 seconds, continuing to a maximum of 5 minutes. A successful observation clears the failure counter. `systemd` is a separate process-level watchdog and restarts the agent after a process failure.

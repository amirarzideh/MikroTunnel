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

`desired_state` is either `enabled` or `disabled`. `actual_state` is a provider observation: `unknown`, `pending`, `up`, `down`, `error`, or `missing`.

Every state transition creates an immutable operation record. This allows a future dashboard and MikroPanel to report pending or failed networking work accurately rather than claiming that a request immediately succeeded on Linux.

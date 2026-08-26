# 2026-08-26: Acknowledge auth provider daemon configuration in shared status

- **Status:** Accepted
- **Date:** 2026-08-26
- **Supersedes:** None
- **Superseded by:** None

## Related issues

None.

## Related ODPs

None.

## Context

Each Obot replica launches auth provider daemons lazily and keeps their process state in memory. Auth provider credentials are shared, but a configuration save previously stopped only the daemon on the replica serving the API request. Other replicas could continue using stale credentials and a different OAuth cookie secret, causing intermittent cross-replica login failures.

Credential storage and `AuthProvider` storage use separate persistence layers, and Obot does not otherwise require every replica to run a controller that invalidates local daemons.

## Decision

The leader-elected auth provider controller will calculate a SHA-256 hash over the provider UID, provider spec, and stored credential environment. It will publish that hash in internal `AuthProvider` status together with the credential mutation revision it observed.

Credential mutations will assign an opaque UUID revision to the `AuthProvider`. The internal OAuth cookie secret will be generated only when missing and will be preserved across ordinary saves.

Before forwarding an auth request, every replica will read the current `AuthProvider` and reconcile its process-local daemon under a provider-scoped lock. It will return a cached daemon only when its recorded hash matches acknowledged status. Unacknowledged revisions, generations, or credential hashes fail closed. Credential values are read and hashed only when a daemon must start or be replaced, and shared state is revalidated after daemon startup.

## Rationale

Acknowledged shared status gives all replicas one authoritative configuration without adding a distributed daemon registry or an every-replica watch controller. Per-request resource reads close the stale-serving window, while hash comparison keeps secrets off the normal cached-daemon path. Provider-scoped locking prevents duplicate local launches without allowing a slow provider to block unrelated providers.

Preserving the cookie secret also prevents unrelated saves from invalidating OAuth flows already in progress and reduces risk during mixed-version rollouts.

## Consequences

- Auth provider URL resolution performs one current shared-storage read before using a local daemon.
- A replica can retain an idle stale daemon, but it cannot forward another request to it without reconciliation.
- Authentication is temporarily unavailable while a credential revision or spec generation is not yet acknowledged by the controller.
- Daemon configuration changes are applied lazily on each replica's next auth request.
- Model provider daemons remain outside this coordination protocol.

## References

- [`pkg/authprovider/configuration.go`](../pkg/authprovider/configuration.go)
- [`pkg/controller/handlers/provider/provider.go`](../pkg/controller/handlers/provider/provider.go)
- [`pkg/gateway/server/dispatcher/dispatcher.go`](../pkg/gateway/server/dispatcher/dispatcher.go)

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

Credential mutations will assign an opaque UUID revision to the `AuthProvider`. Every configuration save will generate a new internal OAuth cookie secret.

Before forwarding an auth request, every replica will read the current `AuthProvider` and stored credential environment, then reconcile its process-local daemon under a provider-scoped lock. It will return a cached daemon only when its recorded hash matches both the hash computed from the current resource and credentials and the hash in acknowledged status. Unacknowledged revisions, generations, or configuration hashes fail closed, and shared state is revalidated after daemon startup.

## Rationale

Acknowledged shared status gives all replicas one authoritative configuration without adding a distributed daemon registry or an every-replica watch controller. Per-request resource and credential reads close both the stale-serving window and the cross-store partial-write window, at the cost of an additional credential query, decryption, and hash calculation. Provider-scoped locking prevents duplicate local launches without allowing a slow provider to block unrelated providers.

Continuing to rotate the cookie secret on every configuration save preserves the existing security behavior. The acknowledged revision and configuration hash ensure that replicas reconcile their daemons to the newly generated secret before serving subsequent authentication requests.

## Consequences

- Auth provider URL resolution reads the current shared resource and stored credential environment before using a local daemon.
- A replica can retain an idle stale daemon, but it cannot forward another request to it without reconciliation.
- Authentication is temporarily unavailable while a credential revision or spec generation is not yet acknowledged by the controller.
- Daemon configuration changes are applied lazily on each replica's next auth request.
- Saving an auth provider configuration invalidates OAuth flows that are already in progress.
- Model provider daemons remain outside this coordination protocol.

## References

- [`pkg/authprovider/configuration.go`](../pkg/authprovider/configuration.go)
- [`pkg/controller/handlers/provider/provider.go`](../pkg/controller/handlers/provider/provider.go)
- [`pkg/gateway/server/dispatcher/dispatcher.go`](../pkg/gateway/server/dispatcher/dispatcher.go)

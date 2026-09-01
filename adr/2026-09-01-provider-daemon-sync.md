# 2026-09-01: Sync auth and model provider daemon configuration across replicas

- **Status:** Accepted
- **Date:** 2026-09-01
- **Supersedes:** None
- **Superseded by:** None

## Related issues

https://github.com/obot-platform/obot/issues/7667

## Related ODPs

None.

## Context

Auth providers and model providers need to be restarted in order to use the latest configuration.
When running Obot with more than one replica, changing the auth or model provider configuration only
caused the provider to restart on the replica that handled the change request. Other replicas
would be stuck running the provider with the older configuration. This could manifest in a number
of different ways, depending on what was changed.

## Decision

Some of the responsibilities for configuring and deconfiguring providers were moved from the API
handlers to a controller. The controller that does it also updates a new ProviderDaemonSync singleton
that is watched by a controller that runs in all replicas. When they see the ProviderDaemonSync change,
they stop the outdated provider daemon, so that it restarts with the new configuration the next time
it needs to serve a request.

## Rationale

While it would have been possible to do this without moving it to controllers, it would have added
latency to every API request on the auth path, since we would need to check the running provider's
configuration against what is stored in the database on every request.

## Consequences

This doesn't affect future work all that much, just a bug fix that caused an architectural change.
The new controller that runs on every replica could be useful for other things in the future.

## References

N/A.

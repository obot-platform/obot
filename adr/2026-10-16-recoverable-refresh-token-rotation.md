# 2026-10-16: Recoverable MCP refresh-token rotation

- **Status:** Accepted
- **Date:** 2026-10-16
- **Supersedes:** None
- **Superseded by:** None

## Related issues

- https://github.com/obot-platform/obot/issues/8179

## Related ODPs

None.

## Context

Deleting a refresh token before issuing its replacement strands clients when requests overlap, storage fails, or responses are lost.

## Decision

Keep one OAuthToken record per grant. New refresh tokens contain its stable ID and a random secret. An update guarded by resourceVersion atomically records the current hash, previous hash, a fixed 60-second retry deadline, and the current token encrypted under a key derived from the previous bearer token using HKDF-SHA-256 and AES-GCM. Legacy tokens upgrade their existing record on first refresh.

During the retry window both tokens return the same successor, after current authorization checks. Requests never extend the window or rotate again within it. Deleting the record revokes the grant; refresh never recreates it.

## Rationale

A single atomic update avoids transactions across token records and orphaned successors. Deriving the recovery key from the presented bearer secret supports recovery across replicas and restarts without storing plaintext credentials or requiring an encryption-provider configuration.

## Consequences

A stolen predecessor can recover the successor during the retry window. Clients must retry within that window. Only one encrypted recovery value is retained per grant, overwritten at the next rotation or removed with the grant. Old servers cannot process the new opaque token format, so deployment and rollback require coordinated versions or a staged compatible rollout.

## References

- [Refresh handling](../pkg/api/handlers/mcpgateway/oauth/token.go)
- [Rotation and recovery](../pkg/api/handlers/mcpgateway/oauth/refresh_token.go)

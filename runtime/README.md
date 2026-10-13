# Claude self-hosted runtime (experimental)

Claude Environments is **experimental** and disabled by default. The user and
operator documentation lives in the documentation site:

- [Overview and request flow](../docs/docs/claude-environments/overview.md)
- [Cluster requirements and GKE/EKS/AKS compatibility](../docs/docs/claude-environments/cluster-requirements.md)
- [Helm installation, images, and storage](../docs/docs/claude-environments/installation.md)
- [Environment profiles, account mapping, and Claude Tag](../docs/docs/claude-environments/administration.md)
- [Egress policy, injected secrets, and audit logging](../docs/docs/claude-environments/security.md)
- [Session lifecycle, maintenance, and troubleshooting](../docs/docs/claude-environments/operations.md)

This directory contains the Obot-side orchestrator hooks. Each Obot replica
supervises a Claude orchestrator child for each environment; no standalone
orchestrator Deployment is installed. Session runners and Substrate component
images are maintained in [substrate-images](https://github.com/obot-platform/substrate-images).

## Runtime contract

Obot admits session-bound work orders after validating identity and current
access. It serializes admission across replicas and retains non-secret work-order
tombstones to prevent redelivery from recreating deleted executions. A session
has a persistent Obot record, with separate executions for reconnects. Substrate
owns the actors and workspace snapshots.

The environment key stays in Obot and its orchestrator child. The session wrapper
preserves Claude's stdin and fd 3, receives MCP URLs with placeholder headers,
and relies on the egress gateway to inject credentials. The runner cannot obtain
Obot keys directly from the mTLS credential-provider endpoint.

On resume, Obot restores with the previous compatible template before switching
the suspended actor to the current template and supplying fresh execution
credentials. The temporary golden actor must not register with Claude or consume
a work order. Keep image and snapshot compatibility in mind when updating this
contract.

The supervisor checks polling progress every 15 seconds, restarts a stalled child
after two minutes, allows five seconds for process-group shutdown, then forces
termination if necessary and retries after five seconds. Other environments and
already running sessions are independent of that child.

Slack metadata is reported by the authenticated runner and filled only when an
individual field is absent. An explicit `false` for mention status is preserved.
This metadata is for display, never identity authorization.

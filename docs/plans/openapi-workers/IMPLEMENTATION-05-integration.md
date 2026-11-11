# I05R Final Integration and Kubernetes Acceptance

Date: 2026-11-10 10:05 Asia/Shanghai. Status: **implementation and isolated acceptance complete**. Production, real enterprise credentials, and live business systems remain out of scope as required.

## Integrated scope

- Merged the four feature slices into this worktree:
  - I01 Python OpenAPI Worker;
  - I02 Go Worker protocol, resolver/executor, fixed backend, MCP JSON bridge, and service handoff;
  - I03 raw-prefix HTTP proxy, dedicated authorization, operation matcher, response writer, and documentation API/UI;
  - I04 PostgreSQL execution budgets, typed leases, bounded IO, audit, and migrations.
- Wired `Services.OpenAPIWorkerExecutor`, `Services.OpenAPIBudgetManager`, and `APIServer.HandleAPIProxy`.
- Added the `MCPRuntimeBackend` environment override so `openapi-worker` does not fall back to Docker in Kubernetes.
- Added `GET /api/vmcps/{vmcp_id}/api-proxy`, the vMCP HTTP API documentation dialog, four locales, and the public documentation section.
- Added an existing-Namespace Kubernetes example with two stateless Obot replicas, two stateless Worker replicas, Services, PDBs, ConfigMap, and a synthetic Secret template.
- Kept PostgreSQL as the only shared state store. The example creates no Namespace, ClusterRole, ClusterRoleBinding, Role, RoleBinding, ServiceAccount, Worker database volume, or Docker socket mount.

## Integration fixes found by real acceptance

- Set Obot `workingDir: /tmp` because Kinm generates `apiserver.local.config` relative to the working directory.
- Set `fsGroup: 10001` so a non-root Obot replica can read mounted mTLS Secret files.
- Stopped the final apply of the example Secret from overwriting generated synthetic integration values.
- Imported the local Obot image by tag rather than a host RepoDigest that K3s containerd did not retain.
- Tightened the Go Executor deadline to `lease.FenceDeadlineUnixMs-1` after acquiring the typed lease, implementing the Obot and Worker double deadline fence.
- Restored all pre-existing `pkg/services/config_test.go` regressions while retaining the new backend environment override test.

## Verification

See `docs/plans/openapi-workers/evidence/I05R-VERIFICATION.md`.

## Handoff and boundaries

- All implementation and isolated acceptance work remains uncommitted as required.
- The K3s cluster, generated certificates, temporary binaries, and synthetic Secret values are disposable acceptance artifacts and are not deliverables.
- Real enterprise authorization exchange, production TLS entry points, production capacity, real business APIs, and production deployment are `UNVERIFIED` and intentionally were not accessed.

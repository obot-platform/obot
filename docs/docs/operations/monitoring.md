---
title: "Monitoring"
---

Monitor Obot, its external database, and the workloads it launches. Gateway usage reports describe user activity; they do not replace infrastructure monitoring.

## What to monitor

| Component | Signals to collect |
|---|---|
| Obot application | `/api/healthz`, request failures, pod/container restarts, CPU and memory pressure |
| External PostgreSQL | Availability, connection usage, storage growth, and backup success |
| Hosted MCP and agent workloads | Pending or failed pods, restarts, resource limits, and failed tool calls |
| Persistent and object storage | Capacity, access failures, and backup/export failures |
| Private-network tunnels | Client connectivity and a representative permitted request to the private service |
| Gateway traffic | MCP and model audit outcomes, usage, and expected user attribution |

## Establish checks and alerts

1. Collect application and workload logs using the logging system for your Docker host or Kubernetes cluster.
2. Monitor the Obot health endpoint and the public ingress separately. A healthy process does not prove that clients can reach it through TLS and authentication.
3. Alert on repeated restarts, unavailable storage or databases, resource pressure, and failed exports or backups.
4. Run representative permitted requests to check the upstream services that matter to your installation. Use a dedicated account with the appropriate grants.
5. Review [MCP audit data](../security/audit-data.md) and [model usage](../llm-gateway/audit.md) for failures and unexpected activity.

Choose alert thresholds from measured workload behavior. See [Capacity](./capacity.md) for sizing and [High availability](./high-availability.md) for replica and failure planning.

## Check the failing component {#check-the-failing-component}

Use the [Troubleshooting symptom table](./troubleshooting.md#locate-the-failure) to investigate a failed check. For failed image pulls, check [image pull secrets](../configuration/image-pull-secrets.md). Remove credentials and sensitive payloads before sharing logs.

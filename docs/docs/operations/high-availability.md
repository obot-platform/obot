---
title: "High availability"
---

High availability requires the Obot application, database, and storage to remain available together. Configure and test each layer before adding replicas.

## Application replicas and storage {#application-replicas-and-storage}

The [Kubernetes deployment guide](../installation/kubernetes-deployment.md#high-availability) describes `replicaCount`, external PostgreSQL, and shared published-workflow storage. Use object storage or a ReadWriteMany Obot data volume when multiple replicas need the same artifacts. A shared ReadWriteOnce claim is not a multi-replica storage solution.

## Failure behavior {#failure-behavior}

[Tunnel peers](../functionality/mcp-tunnels.md#multiple-obot-replicas) route requests to connected tunnel clients across replicas. Failed in-flight requests are not replayed automatically. Peer-token rotation can temporarily interrupt tunneled traffic during a rolling update.

Validate application restart, workload rescheduling, database availability, and storage recovery in a staging environment. Keep [backup and recovery](./backup.md) procedures separate from availability configuration.

## Plan each layer

| Layer | Required planning |
|---|---|
| Obot replicas | Configure the Helm replica count, capacity, and ingress routing. Review pod placement so one node failure does not remove every replica. |
| PostgreSQL | Use an external production database with its own availability and recovery plan. Additional Obot replicas do not replicate the database. |
| Published workflow files | Use object storage or shared ReadWriteMany storage accessible to all Obot replicas. |
| MCP and agent workloads | Assess their own restart behavior and persistent storage. More Obot replicas do not make each workload highly available. |
| Tunnel clients | Operate clients where they can reach the private service; test client and Obot replica loss. |

## Verify failover

In a staging environment, establish a client connection and perform a read-only tool call. Replace an Obot pod, reconnect, and verify a new call and its audit record. Separately test database failover and access to a published workflow from each replica. Inspect failed in-flight requests; do not assume the gateway retries them.

For workload sizing, see [Capacity](./capacity.md). For recovery after data loss, see [Backup and recovery](./backup.md).

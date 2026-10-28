---
title: "Capacity"
---

Size Obot and its hosted workloads separately. A higher Obot replica count does not make every backing MCP server multi-replica.

## Application replicas and storage {#application-replicas-and-storage}

See [High availability](./high-availability.md#application-replicas-and-storage) for this configuration.

## Hosted workload capacity

Use [scheduling configuration](./capacity.md#server-scheduling-configuration) and [MCP Kubernetes settings](../configuration/mcp-deployments-in-kubernetes.md) to configure affinity, tolerations, requests, limits, and maximums. Per-catalog resource requirements can override corresponding defaults. Each MCP deployment currently has one replica.

Capacity depends on simultaneously active per-user deployments, shared servers, request volume, and audit retention. Measure a representative workload; the installation guide's minimum CPU and memory are not a throughput guarantee. Review database connection-pool settings as replica count grows.

## Failure behavior {#failure-behavior}

See [High availability](./high-availability.md#failure-behavior) for this configuration.

## Configure workload placement

Scheduling settings apply only to Kubernetes deployments. They do not apply to Docker.

Server scheduling configures pod placement for MCP server deployments in Kubernetes. These settings map directly to Kubernetes Deployment spec fields and control where and how MCP server pods run.

Use this feature to:

- Control which nodes MCP servers run on
- Define which taints pods can tolerate
- Set resource requests and limits
- Align deployments with cluster topology and capacity planning

All settings are applied to `spec.template.spec` of Kubernetes Deployments. Changes take effect on the next deployment or pod restart.

Open **Platform > MCP Config** on a Kubernetes installation. Adjust affinity, tolerations, and resource settings, then save. Settings supplied through Helm are read-only in the UI; change the corresponding Helm values instead.

## Configuration {#server-scheduling-configuration}

### Affinity {#server-scheduling-affinity}

Defines the affinity field for pods in every MCP deployment. This value sets `spec.template.spec.affinity` on Kubernetes deployments and must be a valid [Affinity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#affinity-v1-core) object.

See the [Kubernetes affinity documentation](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#affinity-and-anti-affinity) for details.

### Tolerations {#server-scheduling-tolerations}

Defines the tolerations field for pods in every MCP deployment. This value sets `spec.template.spec.tolerations` on Kubernetes deployments and must be a valid list of [Toleration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#toleration-v1-core) objects.

See the [Kubernetes taints and tolerations documentation](https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/) for details.

### Resource Limits & Requests {#server-scheduling-resource-limits--requests}

Defines the CPU and memory requests and limits for pods in every MCP deployment.

See the [Kubernetes resource management documentation](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/#requests-and-limits) for details.

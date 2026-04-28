---
title: Server Scheduling
---

# Server Scheduling

:::note
This feature is only available for Kubernetes deployments. It does not apply when running Obot with Docker.
:::

Server Scheduling configures pod scheduling behavior for MCP server deployments in Kubernetes. These settings map directly to Kubernetes Deployment spec fields and control where and how MCP server pods run.

Use this feature to:

- Control which nodes MCP servers run on
- Define which taints pods can tolerate
- Set resource requests and limits
- Align deployments with cluster topology and capacity planning

All settings are applied to `spec.template.spec` of Kubernetes Deployments. Changes take effect on the next deployment or pod restart.

To access this feature, navigate to **MCP Management > Server Scheduling**.

## Configuration

### Affinity

Defines the affinity field for pods in every MCP deployment. This value sets `spec.template.spec.affinity` on Kubernetes deployments and must be a valid [Affinity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#affinity-v1-core) object.

See the [Kubernetes affinity documentation](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#affinity-and-anti-affinity) for details.

### Tolerations

Defines the tolerations field for pods in every MCP deployment. This value sets `spec.template.spec.tolerations` on Kubernetes deployments and must be a valid list of [Toleration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#toleration-v1-core) objects.

See the [Kubernetes taints and tolerations documentation](https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/) for details.

### Resource Limits & Requests

Defines the CPU and memory requests and limits for pods in every MCP deployment.

See the [Kubernetes resource management documentation](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/#requests-and-limits) for details.

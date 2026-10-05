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

## Configuration {#configuration}

Continue to [Configuration](../operations/capacity.md#server-scheduling-configuration).

### Affinity {#affinity}

Continue to [Affinity](../operations/capacity.md#server-scheduling-affinity).

### Tolerations {#tolerations}

Continue to [Tolerations](../operations/capacity.md#server-scheduling-tolerations).

### Resource Limits & Requests {#resource-limits--requests}

Continue to [Resource Limits & Requests](../operations/capacity.md#server-scheduling-resource-limits--requests).

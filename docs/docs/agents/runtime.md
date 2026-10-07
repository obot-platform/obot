---
draft: true
title: "Configure agent runtimes"
---

Configure the runtime and storage for Obot Agent. Its workspace files and published workflow artifacts are stored separately.

| Workload/data | Configuration |
|---|---|
| Obot Agent workspace | Kubernetes `mcpServerDefaults.storageClassName` and `nanobotWorkspaceSize`; otherwise workload-local files can disappear on replacement |
| Published workflow packages | Object storage or the persistent Obot `/data` volume |

Use [Kubernetes workload isolation](../configuration/mcp-deployments-in-kubernetes.md) to understand namespace policy, pod admission, and runtime classes. Validate the effective policy on your agent workloads; a policy documented for MCP server pods is not a blanket guarantee about every deployment.

For durable data, follow [Persistent Storage](../installation/kubernetes-persistent-storage.md) and [Workflow Sharing storage](./workflows.md#workflow-sharing-operational-requirements). Test workload replacement and recovery before scheduling important work.

## Configure and verify the runtime

1. Enable Obot Agent using the [availability requirements](./availability.md#availability-and-prerequisites) and configure its MCP runtime.
2. Ensure the workload can reach its configured Obot gateway endpoints and any additional services the agent needs. Review effective network policies on the actual workload.
3. Configure persistent storage for files that must survive workload replacement. A persistent workspace does not replace storage for published workflow packages.
4. In a test environment, run a small model request, call a read-only MCP tool, and create a test workspace file. Replace the workload and verify the expected file survives and gateway access still works.

Use [Monitoring](../operations/monitoring.md) for health and resource checks. Diagnose pod scheduling, image pulls, and storage failures through [Troubleshooting](../operations/troubleshooting.md).

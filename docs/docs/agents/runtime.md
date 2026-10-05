---
title: "Configure agent runtimes"
---

Configure the runtime for the specific agent workload. Obot Agent workspaces, Hosted Agents pool volumes, and published workflow artifacts are different stores.

| Workload/data | Configuration |
|---|---|
| Obot Agent workspace | Kubernetes `mcpServerDefaults.storageClassName` and `nanobotWorkspaceSize`; otherwise workload-local files can disappear on replacement |
| Hosted Agents sandbox | Kubernetes backend, enabled Hosted Agents feature, compatible sandbox images and Pod Security Admission level |
| Hosted Agents pool storage | `OBOT_SERVER_HOSTED_AGENTS_STORAGE_CLASS_NAME`; each pool uses a ReadWriteOnce volume, with `WaitForFirstConsumer` recommended |
| Published workflow packages | Object storage or the persistent Obot `/data` volume |

Hosted Agents use the MCP namespace but have their own runtime settings, including image pull policy and cleanup image. See the [configuration reference](../configuration/server-configuration.md). Do not configure the development `fake` backend as production hosting.

Use [Kubernetes workload isolation](../configuration/mcp-deployments-in-kubernetes.md) to understand namespace policy, pod admission, and runtime classes. Validate the effective policy on your agent workloads; a policy documented for MCP server pods is not a blanket guarantee about every deployment.

For durable data, follow [Persistent Storage](../installation/kubernetes-persistent-storage.md) and [Workflow Sharing storage](./workflows.md#workflow-sharing-operational-requirements). Test workload replacement and recovery before scheduling important work.

## Configure and verify the runtime

1. Choose Obot Agent or Hosted Agents using the [availability requirements](./availability.md#availability-and-prerequisites). Enable the corresponding feature and configure its supported backend.
2. For Hosted Agents, use a compatible sandbox image and configure the Kubernetes namespace policy, image access, and storage required by that image. The development `fake` backend is not a production sandbox.
3. Ensure the workload can reach its configured Obot gateway endpoints and any additional services the template needs. Review effective network policies on the actual workload.
4. Configure persistent storage for files that must survive workload replacement. A persistent workspace does not replace storage for published workflow packages.
5. In a test environment, run a small model request, call a read-only MCP tool, and create a test workspace file. Replace the workload and verify the expected file survives and gateway access still works.

Use [Monitoring](../operations/monitoring.md) for health and resource checks. Diagnose pod scheduling, image pulls, and storage failures through [Troubleshooting](../operations/troubleshooting.md).

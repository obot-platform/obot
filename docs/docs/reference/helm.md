---
description: Helm chart values for Kubernetes deployments.
title: Helm values
---

The Helm chart reference is published at [charts.obot.ai](https://charts.obot.ai/). Use the values for the exact chart version you deploy.

| Configuration area | Detailed guide |
|---|---|
| Obot replicas, ingress, secrets, and environment | [Kubernetes Deployment](../installation/kubernetes-deployment.md) |
| MCP namespace, pod security, resources, and RuntimeClass | [MCP Servers in Kubernetes](../configuration/mcp-deployments-in-kubernetes.md) |
| Obot data volume and StorageClass | [Persistent Storage](../installation/kubernetes-persistent-storage.md) |
| Private image registries | [Image Pull Secrets](../configuration/image-pull-secrets.md) |
| Domain egress provider | [MCP Server Egress Control](../configuration/mcp-server-egress-control.md) |
| Encryption providers | [Encryption](../security/credentials.md) |

Put sensitive values under the chart's `secret` settings rather than `config`. For reviewed production configuration, retain chart version and values alongside the deployment's change history. See [Server Configuration](../configuration/server-configuration.md) for environment-variable descriptions; not every development-only variable has a dedicated Helm value.

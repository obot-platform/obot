---
title: "Troubleshooting"
---

Start with the component that failed. Record the time, affected user, endpoint, and any request or correlation ID before changing configuration.

## Locate the failure

| Symptom | Starting point |
|---|---|
| Obot is unreachable | Container/pod state, ingress/TLS, and `/api/healthz` |
| Sign-in fails | [Authentication troubleshooting](../configuration/auth-providers.md#enabling-authentication-troubleshooting) |
| Hosted MCP pod does not start | Pod events, resource capacity, admission policy, and [image pull secrets](../configuration/image-pull-secrets.md#troubleshooting) |
| Obot data PVC stays pending | [Persistent storage validation](../installation/kubernetes-persistent-storage.md#validation) |
| Remote MCP call fails | [Gateway troubleshooting](../mcp-gateway/troubleshooting.md) |
| Domain allowlist does not apply | [Egress provider verification](../configuration/mcp-server-egress-control.md#verify-the-setup) |
| Model call fails | Model policy, provider credentials, exact model identifier, and [LLM troubleshooting](../llm-gateway/troubleshooting.md) |


## Collect diagnostics

For Docker, inspect `docker ps` and `docker logs <obot-container>`. For Kubernetes, inspect pod state, events, and logs in both the Obot and MCP namespaces. Check ingress/TLS when the public endpoint fails but the application is healthy internally. Check the external database when Obot cannot start or persist changes.

Correlate application logs with MCP or model audit records by timestamp and resource. A successful `/api/healthz` response does not prove that a provider credential, model grant, MCP component, or tunnel is working.

## Verify the repair

Repeat the same read-only request under the affected user's account. Verify its result and the corresponding audit entry. For a storage problem, verify the expected files remain available after workload replacement in a test environment.

Remove credentials and sensitive payloads before sharing diagnostics. See [Monitoring](./monitoring.md) for routine observations and alerts.

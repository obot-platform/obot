---
title: Sessions, maintenance, and troubleshooting
description: Verify a deployment, operate session lifecycle and retention, and diagnose experimental Claude Environments.
---

:::warning Experimental
Snapshots and APIs may change between releases. Suspend sessions and back up durable state before runtime or node changes. Keep important work in source control; a snapshot is not a substitute for a repository backup.
:::

## Acceptance checks

Run these checks before describing a cluster or provider as compatible:

1. Install into the intended cluster and confirm bootstrap Jobs complete, certificate APIs are served, projected certificates mount, and all infrastructure pods become Ready.
2. Create an environment with a small worker pool and a profile granting a test user access. Confirm the worker lands on a node with the matching atelet.
3. Map the account, start a fresh Claude session, and confirm its identity, environment, and running state in Obot.
4. Call an allowed vMCP, deny an unauthorized user, and check that an unlisted egress destination is blocked. Verify audit metadata and auditor-only payload access.
5. Suspend the session, confirm a snapshot is written, and resume it. Verify workspace files, vMCP access, and that Obot retains the same session record.
6. Change an allowed host or injected secret and verify a fresh request from the running session observes the new configuration after reconciliation.
7. Test Claude Tag separately if used, including its service profile and tool credentials.
8. Delete the test session/environment and verify associated actors, templates, credentials, and worker resources are removed. Test snapshot deletion permissions too.

API discovery alone cannot establish node projection, gVisor, storage, or network compatibility. This procedure has not yet been completed for Obot on EKS or AKS.

## Session lifecycle and retention

**My Claude Sessions** lists the current user's sessions. Administrators can list all sessions, inspect their status and associated Obot user, and suspend or delete them. Repository URLs/revisions, last-used time, and available Slack sender, handle, channel, and thread links are displayed. Metadata may be absent until the runner reports it; Slack fields retain their first observed values across resumes.

A suspended session normally indicates an idle runner whose workspace has been checkpointed. Its idle timeout defaults to 5 minutes. Resuming starts a new execution from the saved workspace. A Pending session may instead be waiting for worker capacity, an image, a certificate, or another execution's suspension.

Session cleanup defaults to **30 days after the latest execution finishes**, not 30 days after initial creation. A session active for 31 days is not deleted just because of its original creation time. Cleanup waits for a finished, suspended session. Configure retention per environment/profile; **`-1` disables session-retention cleanup**. This does not disable access-revocation or stuck-runner cleanup. Zero is not a valid effective session-retention value.

Older completed executions are removed after a newer execution safely suspends. The latest execution is retained for reconnects until retention expires or the session is deleted. Cleanup uses finalizers to remove actors, tags, templates, and credentials without deleting tags still needed by another actor. An orphan sweep also handles interrupted cleanup, with a 15-minute grace period. Resource deletion is asynchronous; do not remove finalizers merely to hide a stuck item.

## Orchestrator health

Each Obot replica supervises an orchestrator child per environment. It checks polling progress every 15 seconds and restarts that environment's child after about two minutes without successful progress. An empty work queue is healthy. Running sessions continue independently of this poller.

If no new session appears, confirm Claude selected the intended environment, the environment key is valid, account mapping/profile access is correct, and Obot logs show work orders or poller recovery. Obot's UI reflects admitted sessions, not every request made in Slack or Claude.

## Diagnostics

Use the namespace configured in your installation where it differs from these examples. Commands are read-only:

```nu
kubectl get pods -n ate-system -o wide
kubectl get pods -n podcertificate-controller-system
kubectl get workerpools -n obot-agents
kubectl get pods -n obot-agents -o wide
kubectl get events -n ate-system --sort-by=.lastTimestamp
kubectl get events -n obot-agents --sort-by=.lastTimestamp
kubectl logs -n obot deployment/obot-obot --since=15m
kubectl logs -n ate-system deployment/ate-api-server --since=15m
```

Replace `obot-obot` with your release's Obot Deployment name. To inspect a particular pod, use `kubectl describe pod POD_NAME -n NAMESPACE`. Avoid posting raw environment keys, session tokens, or credential material with diagnostic output.

| Symptom | Check |
|---|---|
| Bootstrap waits for trust bundles; unknown certificate resource | Control-plane API discovery and certificate-controller events. Helm cannot supply missing APIs. |
| `MountVolume.SetUp failed: unimplemented` | Node projection support, not only control-plane version. On GKE 1.36, replace nodes after enabling the APIs. |
| Worker pod Pending | Available node CPU/memory, selectors, taints, image pull failures, and whether atelet runs on the selected node. Requests equal configured worker limits. |
| Session Pending / `ResourceExhausted` | Worker capacity and Substrate actor state. Actors are not individual pods; stale/resuming executions can consume capacity even when the UI shows no running session. Inspect before deleting anything. |
| Obot says Running, Claude says no runner | A running actor does not prove the Claude process registered. Inspect Obot/runner failures, environment key, outbound Anthropic connectivity, and the latest execution. |
| Runner integration failed / HTTP 403 | Mapped user is active, environment profile matches, effective vMCP list is allowed by the vMCP itself, and Claude Tag has its own permissions if applicable. |
| Suspension/resume fails | Snapshot bucket permissions, database health, image compatibility, and retained runtime state. A control-plane restart does not repair incompatible snapshots. |
| Removed hostname still appears accessible | Wait for reconciliation/cache expiry, make a fresh request, and check other matching profiles or Allow all web destinations. Internal Obot and Anthropic remain allowed. |
| No audit body | Auditor membership, request completion/batch delay, truncation, or dropped capture. A WebSocket transcript is not captured. |

## Maintenance, backup, and shutdown

Back up Obot's database and encryption configuration, Substrate's database, snapshot objects, and Substrate's signing pools together. Storage credentials and signing material are not interchangeable with session snapshots.

Before changing worker images, changing node selectors, reducing capacity, or replacing nodes, suspend affected sessions and wait for successful checkpointing. Plan autoscaler and node-upgrade disruption rather than relying on forced eviction to save work. Avoid Spot/preemptible workers for sessions whose state matters. Follow the pinned upstream [upgrade runbook](https://github.com/agent-substrate/substrate/blob/531c935370b9a14463c6588a63102922daebb810/docs/upgrade.md); image tags alone do not establish snapshot compatibility.

Deployment image defaults affect new environments only. Update existing environments explicitly and test a fresh session plus resume. Changing chart-managed storage settings rolls affected components; changes to an independently managed credentials Secret can require a restart.

To retire the feature, delete environments in Obot first and wait for their sessions and worker pools to finish cleanup. Then set `substrate.enabled=false` in your Helm upgrade. Disabling/uninstalling first removes the controllers needed to complete application cleanup.

Helm retains namespaces, CRDs, signing material, and database PVCs to preserve durable state. Review retained resources and their ownership before manually deleting them. PVC deletion follows the volume reclaim policy, so `Retain` volumes need separate cleanup. Uninstalling alone is not a complete purge of snapshots or database records.

## Upgrading from Hosted Agents

The old Hosted Agents backend/UI is removed. At startup, Obot cleans up its legacy Kubernetes deployments, jobs, pods, ReplicaSets, secrets, services, quotas, and pool PVCs in the configured MCP namespace, selected by `obot.ai/managed-by=obot-agent-backend`. Legacy pool PriorityClasses must also belong to that namespace. Normal MCP resources are preserved.

Cleanup completes before retired Hosted Agent tables and stored sandbox credentials are removed. Errors/timeouts fail startup and retry on restart. **Legacy pool data is discarded.** Back it up before upgrading if needed; it is not migrated into Claude Environments.

---
title: Upgrades and rollback
---

Use explicit image and chart versions for production upgrades. Review the target release's changes before replacing the running application.

## Prepare

1. Record the current image digest/tag, chart version, values, and external dependencies.
2. Read release notes and relevant [compatibility guidance](../reference/compatibility.md), including vMCP and agent feature transitions.
3. Take and test an appropriate [backup](./backup.md).
4. Rehearse the upgrade against a restored staging installation. Confirm authentication, representative gateway requests, audit records, artifacts, and agent persistence.

## Apply and verify

For Kubernetes, update the selected chart version and values through your normal Helm or GitOps deployment process. For Docker evaluation, recreate the container with the selected image and the same persistent data volume and configuration. Do not remove the data volume as part of an image update.

Check the rollout, application health, sign-in, and representative workloads. Review [tunnel peer-token behavior](../functionality/mcp-tunnels.md#multiple-obot-replicas) before changing peer secrets during a multi-replica rollout.

## Rollback

Do not assume that changing the image tag or running a Helm rollback reverses database or resource migrations. Confirm downgrade compatibility for the exact release first. When it is not established, use the recorded prior application/configuration with a compatible pre-upgrade data restore in a rehearsed recovery procedure.

A data restore can lose writes made after the backup. Decide how to stop traffic and scheduled work and how to handle that recovery point before proceeding with production changes.

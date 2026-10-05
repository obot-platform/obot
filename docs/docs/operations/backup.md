---
title: Backup and recovery
---

Back up the stores that hold your installation's state, together with the configuration and key access needed to read them. Audit export alone is not a recoverable Obot backup.

## Backup scope

| Store | Preserve |
|---|---|
| PostgreSQL | Platform configuration, identities, metadata, and retained database-backed audit records |
| Published workflow storage | Object-store contents or the persistent local artifact directory |
| Agent workspaces and hosted-agent pool volumes | Persistent files required by those workloads |
| Deployment configuration | Image/chart versions, Helm values, runtime settings, and references to external secrets |
| Encryption | Custom key material or access to the original cloud KMS keys, protected separately |

Use [persistent storage configuration](../installation/kubernetes-persistent-storage.md) to identify the Obot data volume, agent workspace volumes, and their StorageClasses.

Use your database and storage providers' backup mechanisms. Define an acceptable recovery point and recovery time for your installation, and keep a version record with each backup. Coordinate database and file/object backups so restored metadata does not refer to missing artifacts.

## Recovery exercise

1. Restore into an isolated environment using the recorded Obot and chart versions.
2. Restore the database and relevant object/volume data using the providers' documented restore procedures.
3. Restore secret references and encryption-provider access before starting Obot.
4. Configure the restored environment's hostname, TLS, and authentication callbacks.
5. Confirm sign-in, resource visibility, decryption, a harmless MCP request, and access to a saved artifact or workspace.
6. Keep scheduled workflows and external-action integrations disabled until their behavior is reviewed.

Only treat a backup as usable after testing restoration. See [Data storage and lifecycle](../architecture/data-lifecycle.md) and [Encryption Providers](../security/credentials.md) for coverage boundaries. Infrastructure-specific restore commands depend on your chosen database and storage services.

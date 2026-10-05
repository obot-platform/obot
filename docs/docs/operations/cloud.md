---
displayed_sidebar: sidebar
title: AWS / Azure / GCP
---

Use the standard [Kubernetes deployment](../installation/kubernetes-deployment.md) with your cloud's identity, networking, database, and storage services.

| Platform | Deployment guide | Encryption guide |
|---|---|---|
| AWS | [Amazon EKS](../installation/reference-architectures/02-aws-eks.md) | [AWS KMS](../configuration/encryption-providers/aws-kms.md) |
| Azure | [Azure AKS](../installation/reference-architectures/03-azure-aks.md) | [Azure Key Vault](../configuration/encryption-providers/azure-key-vault.md) |
| GCP | [Google GKE](../installation/reference-architectures/01-gcp-gke.md) | [Google Cloud KMS](../configuration/encryption-providers/google-cloud-kms.md) |

Prepare PostgreSQL, TLS ingress, cloud identity permissions, and the required StorageClasses before installation. Published workflow storage and agent workspace volumes are distinct; use [Data storage and lifecycle](../architecture/data-lifecycle.md) to identify each dependency.

---
title: Overview
slug: /installation/overview
---

This guide helps you choose the right deployment method for your use case. Before selecting a deployment option, it’s useful to understand the core components that make up an Obot installation:

- **Obot Server**: The core application server, distributed as a container image.
- **MCP Server Hosting Platform**: The environment where Obot deploys users’ MCP servers. This matches the platform on which Obot itself is deployed (Docker or Kubernetes).
- **PostgreSQL Database**: The primary database for Obot. PostgreSQL 17 or later is required, along with the [pgvector](https://github.com/pgvector/pgvector) extension.
- **File Storage**: Local filesystem or S3-compatible object storage for files generated or uploaded during chat threads or task runs.

Below you’ll find an overview of the available deployment options, along with system requirements and links to reference architectures.

## Docker Deployment

Docker provides the fastest way to get Obot running on your local machine or a single server for development, testing, and proof-of-concept use cases.

- Simple setup using `docker run`
- Includes a built-in PostgreSQL database and local file storage
- Deploys MCP servers as Docker containers using the host’s Docker socket

For more details, see the [Docker Deployment Guide](/installation/docker-deployment/).

## Kubernetes Deployment

Kubernetes provides the best way to run Obot reliably at scale in production environments.

- Helm chart available at [charts.obot.ai](https://charts.obot.ai/)
- Integrates with cloud-native services such as KMS and S3
- Requires an external PostgreSQL database and external storage

For more details, see the [Kubernetes Deployment Guide](/installation/kubernetes-deployment/).

## Production System Requirements

For production deployments, the following components are required:

- **Kubernetes**: A production-grade Kubernetes cluster with capacity for Obot and the MCP servers it will manage
- **External PostgreSQL database**: PostgreSQL 17 or later with the pgvector extension
- **S3-compatible object storage**: For workspace files and persistent data
- **Encryption provider**: AWS KMS, Google Cloud KMS, or Azure Key Vault
- **Authentication provider**: See our supported [Authentication Providers](/configuration/auth-providers)
- **TLS/SSL certificates**: For secure HTTPS access
- **Backup strategy**: Regular backups for both the database and object storage

## Cloud Platform Reference Architectures

If you plan to deploy Obot on a managed Kubernetes service, these reference architectures provide infrastructure guidance and best practices:

- [GCP GKE Reference Architecture](/installation/reference-architectures/gcp-gke/)
- [AWS EKS Reference Architecture](/installation/reference-architectures/aws-eks/)
- [Azure AKS Reference Architecture](/installation/reference-architectures/azure-aks/)

## Next Steps

1. Choose a deployment method above
2. Follow the corresponding deployment guide
3. [Configure authentication](/configuration/auth-providers)
4. [Set up model providers](/configuration/model-providers)
5. Review the [server configuration options](/configuration/server-configuration)

## Getting Help

- See the [FAQ](/faq/)
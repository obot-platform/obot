---
title: Overview
slug: /installation/overview
---

Obot is a complete platform for building and running AI agents. This guide will help you choose the right deployment method for your use case.

## Obot Components

Obot consists of three main components:

- **Obot Server**: The core application server
- **PostgreSQL Database**: Version 17 or higher with [pgvector](https://github.com/pgvector/pgvector) extension
- **Data Storage**: Local filesystem or S3-compatible storage for workspace files

Obot stores its data under the `/data` path. It also includes the data for the built-in PostgreSQL instance during development. Production deployments require an external PostgreSQL database.

## Deployment Options

### Docker Deployment

**Best for**: Local development, testing, proof-of-concept

Docker provides the fastest way to get Obot running on your local machine or a single server.

- Simple setup with `docker run`
- Ideal for development and evaluation
- Uses built-in PostgreSQL

👉 [Docker Deployment Guide](docker-deployment)

### Kubernetes Deployment

**Best for**: Production deployments, scalability, high availability

Deploy Obot on Kubernetes for production-grade reliability and scalability.

- Helm chart available at [charts.obot.ai](https://charts.obot.ai/)
- Integrates with cloud services (KMS, S3, etc.)
- Requires external PostgreSQL database

👉 [Kubernetes Deployment Guide](kubernetes-deployment)

### Cloud Platform Deployments

**Best for**: Cloud-native deployments with managed services

Deploy Obot on major cloud platforms with platform-specific guidance.

- **Cloud-specific integrations**: Use managed databases, key management, and storage services

For production-ready architectures on specific clouds:

- [GCP GKE Reference Architecture](reference-architectures/gcp-gke)
- [AWS EKS Reference Architecture](reference-architectures/aws-eks)
- [Azure AKS Reference Architecture](reference-architectures/azure-aks)

## System Requirements

### Minimum (Development/Testing)

- **CPU**: 1 cores
- **RAM**: 2 GB
- **Storage**: 10 GB

### Recommended (Production)

- **CPU**: 2+ cores per instance
- **RAM**: 4+ GB per instance
- **Storage**: 50+ GB

## Database Requirements

- **Development**: Built-in PostgreSQL included
- **Production**: External PostgreSQL 17+ required with [pgvector](https://github.com/pgvector/pgvector) extension

## Production Considerations

For production deployments, you should have:

- **External PostgreSQL database**: PostgreSQL 17+ with pgvector extension
- **S3-compatible storage**: For workspace files and data
- **Encryption provider**: AWS KMS, Google Cloud KMS, or Azure Key Vault
- **Authentication**: OAuth, OIDC, or enterprise providers (SAML, LDAP)
- **TLS/SSL certificates**: For secure HTTPS access
- **Backup strategy**: Regular backups of database and storage

## Quick Decision Guide

| Use Case | Recommended Deployment |
|----------|----------------------|
| Local development | [Docker](docker-deployment) |
| Production | [Kubernetes](kubernetes-deployment) |

## Next Steps

1. Choose your deployment method above
2. Follow the deployment guide
3. [Configure authentication](../configuration/auth-providers)
4. [Set up model providers](../configuration/model-providers)
5. Review [server configuration](../configuration/server-configuration)

## Getting Help

- Check [FAQ](../faq)

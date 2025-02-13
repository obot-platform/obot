---
title: Overview
slug: /installation/overview
---

# Overview

## Obot Architecture

Obot is a complete platform for building and running agents. The main data components are:

- Obot server
- Postgres database
- Workspace provider
- Caching directory

By default, the default Obot Docker setup will run a postgres database, and use the local `/data` volume for workspace and caching. This is the volume data you would want to persist.

### Production Considerations

For a production setup, you will want to use an external Postgres database, and an S3-compatible storage provider for the workspace.

To configure Obot to use

## System requirements

### Minimum

We recommend the following for local testing:

- 2GB of RAM
- 1 CPU core
- 10GB of disk space

### Recommended

- 4GB of RAM
- 2 CPU cores
- 40GB of disk space

Along with external Postgres and S3 storage for production use cases.

## Installation Methods

There are several ways to install Obot.

### Docker

Docker is the easiest way to get started with Obot.

The OSS version of Obot image is `ghcr.io/obot-platform/obot:latest`

The Enterprise version of Obot image is `ghcr.io/obot-platform/obot-enterprise:latest`

For a local installation, you can run the following command:

```bash
docker run -d -p 8080:8080 ghcr.io/obot-platform/obot:latest
```

#### With Authentication

```bash
docker run -d -p 8080:8080 -e "OBOT_SERVER_ENABLE_AUTHENTICATION=true" ghcr.io/obot-platform/obot:latest
```

The bootstrap token needed to login as admin will be output to the screen, and can be obtained by running

#### Advanced Configuration

- [Server Configuration](/configuration/general)
- [Workspace Configuration](/configuration/workspace-provider)

```bash
docker logs -f <container_id>
```

### Helm

If you would like to install Obot on a Kubernetes cluster, you can use the Helm chart. We are currently working on the Helm chart and have made it available for testing here: [obot-helm](https://charts.obot.ai/)

## Next Steps

- [Configure Authentication](/configuration/auth-providers)
- [Configure Model Providers](/configuration/model-providers)

# Docker Deployment

Deploy Obot using Docker for local development, testing, and proof-of-concept scenarios.

## Overview

Docker deployment is the fastest way to get Obot running. It's ideal for:

- Local development and testing
- Single-machine deployments
- Proof-of-concept and evaluation
- Small team usage

For production deployments, see [Kubernetes Deployment](kubernetes-deployment).

## Prerequisites

- Docker installed
- 2+ CPU cores and 4GB RAM available
- 10GB disk space

## Quick Start

### Basic Deployment (Built-in PostgreSQL)

Run Obot with the built-in PostgreSQL instance (suitable for development and testing):

```bash
docker run -d \
  --name obot \
  -v obot-data:/data \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -p 8080:8080 \
  -e OPENAI_API_KEY=your-openai-key \
  ghcr.io/obot-platform/obot:latest
```

#### With Authentication

```bash
docker run -d \
  --name obot \
  -v obot-data:/data \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -p 8080:8080 \
  -e OPENAI_API_KEY=your-openai-key \
  -e OBOT_SERVER_ENABLE_AUTHENTICATION=true \
  ghcr.io/obot-platform/obot:latest
```

## Accessing Obot

Once started, access Obot at:

- **Web UI**: http://localhost:8080

### Bootstrap token

The first time you access Obot, you may need the bootstrap token found in the logs:

```bash
docker logs obot
```

You will need to look for an entry like:

```shell
--------------------------------------
| Bootstrap token: <BOOTSTRAP_TOKEN> |
--------------------------------------
```

## Next Steps

1. **Configure Authentication**: Set up [auth providers](../configuration/auth-providers) for secure access
2. **Configure Model Providers**: Configure [model providers](../configuration/model-providers) (OpenAI, Anthropic, etc.)
3. **Set Up MCP Tools**: Deploy [MCP servers](../concepts/mcp-gateway/overview) for extended functionality

## Related Documentation

- [Installation Overview](overview)
- [Kubernetes Deployment](kubernetes-deployment)
- [Server Configuration](../configuration/server-configuration)

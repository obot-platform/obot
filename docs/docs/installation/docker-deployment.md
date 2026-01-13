# Docker Deployment

Deploy Obot using Docker for local development, testing, and proof-of-concept scenarios.

## Overview

Docker deployment is the fastest way to get Obot running. It's ideal for:

- Local development and testing
- Single-machine deployments
- Proof-of-concept and evaluation
- Small team usage

For production deployments, see [Kubernetes Deployment](/installation/kubernetes-deployment/).

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
  -e OBOT_BOOTSTRAP_TOKEN=your-bootstrap-token \
  ghcr.io/obot-platform/obot:latest
```

#### Using a Custom Port

If you want to expose Obot on a different port (e.g., `-p 9999:8080`), you must also set `OBOT_SERVER_HOSTNAME` so that authentication redirects and MCP server connection URLs work correctly:

```bash
docker run -d \
  --name obot \
  -v obot-data:/data \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -p 9999:8080 \
  -e OPENAI_API_KEY=your-openai-key \
  -e OBOT_SERVER_HOSTNAME=localhost:9999 \
  ghcr.io/obot-platform/obot:latest
```

## Accessing Obot

Once started, access Obot at http://localhost:8080.

If you enabled authentication, use your bootstrap token to log in as the owner and set up an authentication provider. If you didn't supply a bootstrap token, a random one will be generated and can be found in the container's logs by searching for "Bootstrap token".

## Next Steps

1. **Configure Authentication**: Set up [auth providers](/installation/enabling-authentication/) for secure access
2. **Configure Model Providers**: Configure [model providers](/configuration/model-providers/) (OpenAI, Anthropic, etc.)
3. **Set Up MCP Servers**: Configure [MCP servers](/functionality/mcp-servers/) for extended functionality

## Related Documentation

- [Installation Overview](/installation/overview/)
- [Kubernetes Deployment](/installation/kubernetes-deployment/)
- [Server Configuration](/configuration/server-configuration/)

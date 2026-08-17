---
title: Architecture
---

# Architecture

Obot is designed to enable organizations to consume MCP servers in an enterprise setting. It consists of four core components: MCP Hosting for running servers, MCP Registry for discovery, MCP Gateway for access control, and Obot Agent for user interaction.

![Obot Platform Architecture](/img/obot-mcp-mgmt.png)

## Key Concepts

- **MCP Clients**: Tools that interact with LLMs and consume MCP servers. These include agents, desktop tools like Cursor, Claude Desktop, VS Code, and Obot Agent.

- **MCP Servers**: Code that implements the MCP specification (tools, prompts, resources) for consumption by clients.

- **MCP Registry**: An index of MCP servers with metadata about how to run them and where to find them.

- **MCP Gateway**: A reverse-proxy that authenticates users, ensures servers are deployed, and forwards requests. See [MCP Gateway](../concepts/mcp-gateway.md) for details.

- **MCP Hosting**: Infrastructure for running MCP server containers (Docker or Kubernetes).

- **LLM Gateway**: A proxy between chat clients and LLMs that enables monitoring and control of LLM communications.

## Authentication Flow

All clients first authenticate with Obot via the configured identity provider. The gateway validates and authorizes the user, obtains the user's stored upstream OAuth token when the target server requires one, and proxies the request to the MCP server.

Key security properties:
- **Gateway**: Handles user authentication, authorization, audit logging, and webhook filters
- **Secret isolation**: Stored OAuth credentials are applied by the gateway and are never exposed as MCP server configuration.

## Data Persistence

- **Database**: Postgres for storing configuration and metadata. In production, this should be hosted independently of the Obot deployment.
- **Published Workflow Storage**: Optional S3, GCS, Azure Blob Storage, or S3-compatible storage for published workflows. If unset, Obot stores published workflows on local disk.
- **Agent State**: Used to store files and other data for Obot Agent. Can be configured to use external volumes for persistence.

## Encryption

Obot uses cloud KMS systems to encrypt data at rest. See [Encryption Providers](../configuration/encryption-providers/overview.md) for configuration options.

## LLMs

Obot operates with a bring-your-own-model philosophy. Multiple providers can be configured to meet organizational requirements. See [Model Providers](../configuration/model-providers.md) for details.

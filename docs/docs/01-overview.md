---
title: Overview
slug: /
---
import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

Obot is an open-source MCP Gateway and AI platform that can be deployed in the cloud or on-prem.

## Getting Started

To quickly try a live demo of the Obot MCP Gateway and chat experience, visit [https://chat.obot.ai](https://chat.obot.ai).

To install Obot yourself, you’ll need access to a Kubernetes cluster. Once that’s ready, run:
```bash
helm repo add obot https://charts.obot.ai
helm install obot obot/obot \
  --set config.OPENAI_API_KEY="<API KEY>"
```
:::tip
You need to replace `<API KEY>` with your [OpenAI API Key](https://platform.openai.com/api-keys).

Setting this is optional, but you'll need to setup a model provider from the Admin UI before using chat.
:::

:::tip
If you’re running on a simple local cluster (e.g., Kubernetes on Docker Desktop), you can port-forward to access Obot at http://localhost:8080:
```
kubectl port-forward svc/obot-obot 8080:80
```
:::

For more installation methods, see our [Installation Guide](/installation/general).

## The Three Parts of Obot
The platform consists of three main components that work together to deliver a comprehensive AI solution.

### 🔌 MCP Gateway
The **MCP Gateway** is where users discover and connect to MCP servers using any MCP client. It provides:

- **Server Discovery** – Browse a catalog of MCP servers tailored to your role and permissions
- **Configuration Management** – Manage all MCP server settings and credentials in one place
- **Upgrade Management** – Receive notifications about available server upgrades and apply them easily
- **Broad Client Support** – Connect with local clients such as Claude Desktop and VS Code or use our hosted Obot Chat
- **OAuth 2.1 Authentication** – Securely authenticate with external services

### 🗣️ Chat
The **Chat Interface** is where users interact with AI through natural, conversational chat. It’s the primary way to ask questions, get answers, and work with connected tools and data. Key features include:

- **Chat Threads** – Keep discussions organized and maintain context over time
- **MCP Server Integration** – Connect to SaaS platforms, APIs, and other tools through [MCP servers](https://modelcontextprotocol.io)
- **Knowledge Integration** – Use built-in RAG to add relevant knowledge to your conversations
- **Tasks** - Create and schedule repeatable tasks that can leverage all the same capabilities as Chat
- **Project-Based Customization** – Tailor AI's behavior to meet your needs with custom instructions, knowledge, and MCP servers at the project level

### ⚙️ Admin
The **Admin Interface** provides comprehensive platform management tools for administrators:

- **Catalog Management** – Create and update MCP server entries using GitOps or the admin portal
- **Server Deployment and Hosting** - Let Obot deploy and host MCP servers to ease your operational burden
- **Access Control Rules** – Define which users and groups can access specific MCP servers
- **Audit Logging** – Track and record all MCP server and client interactions
- **Request Filtering** – Programmatically inspect and reject requests to/from MCP servers for enhanced security and compliance
- **User Management** – Manage users, groups, and access permissions
- **Model Provider Management** – Configure and manage LLM providers and settings for the Chat Interface
- **Centralized Authentication** - Integrate with your existing auth provider to ensure proper user authentication and authorization
- **Monitoring** – View system health metrics and usage analytics

## How They Work Together

These three components create a powerful, integrated AI platform:

1. **Users** interact with Obot projects through the **Chat Interface** and MCP Servers through the **MCP Gateway**.
2. **Users** and **MCP Clients** leverage tools via the **MCP Gateway**
3. **Administrators** manage the entire platform through the **Admin Interface**

## Key Features

- **Self-Hosted**: Deploy on your own infrastructure for complete control
- **MCP Standard**: Built on the open Model Context Protocol for maximum interoperability
- **Enterprise Security**: OAuth 2.1 authentication, encryption, and audit logging
- **Extensible**: Easy integration with custom tools and services

## Next Steps

For detailed installation instructions, please refer to our [Installation Guide](/installation/general).

To understand each component in depth:
- [Chat Interface Concepts](/concepts/chat/overview)
- [MCP Gateway Concepts](/concepts/mcp-gateway/overview)
- [Admin Interface Concepts](/concepts/admin/overview)

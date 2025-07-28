---
title: Overview
slug: /
---
import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

Obot is an open source AI assistant platform that can be deployed self-hosted in the cloud or on-prem. The platform consists of three main components that work together to provide a comprehensive AI agent solution.

To quickly view a demo environment, you can visit our [hosted version](https://chat.obot.ai).

## The Three Parts of Obot

### 🗣️ Chat Interface
The **Chat Interface** is where end users interact with AI agents (called Projects) through conversational chat. This is the primary user-facing component of obot that provides:

- **Projects**: Individual AI assistants that can be customized for specific tasks
- **Threads**: Separate conversations within each project to maintain context
- **Knowledge Integration**: Built-in RAG for connecting agents to your organization's data
- **Tool Integration**: Agents can work with tools, browsers, APIs, and external services through MCP
- **Collaboration**: Share projects with team members and collaborate on AI-powered workflows

### 🔌 MCP Gateway
The **MCP Gateway** implements the Model Context Protocol (MCP) standard, enabling seamless integration between AI agents and external tools and services. This component handles:

- **Tool Catalogs**: Browse and connect to available MCP servers and tools
- **OAuth Flows**: Secure authentication with external services
- **Session Management**: Handle connections between agents and MCP servers
- **Webhook Support**: Receive events and data from external systems
- **Custom Integrations**: Connect your own tools and services through the MCP protocol

### ⚙️ Admin Interface
The **Admin Interface** provides comprehensive platform management capabilities for administrators and power users:

- **User Management**: Manage users, roles, and access control
- **Agent Configuration**: Create and manage base agents and system-wide obots
- **Task Automation**: Set up scheduled tasks and workflows
- **Model Providers**: Configure and manage LLM providers and settings
- **System Configuration**: Configure authentication providers, encryption, and platform settings
- **Monitoring**: View task runs, system health, and usage analytics

## How They Work Together

These three components create a powerful, integrated AI platform:

1. **Users** interact with AI agents through the **Chat Interface**
2. **Agents** leverage tools and services via the **MCP Gateway**  
3. **Administrators** manage the entire platform through the **Admin Interface**

## Key Features

- **Self-Hosted**: Deploy on your own infrastructure for complete control
- **MCP Standard**: Built on the open Model Context Protocol for maximum interoperability
- **Enterprise Security**: OAuth 2.0 authentication, encryption, and audit logging
- **Flexible Deployment**: Kubernetes, Docker, or any cloud provider
- **Extensible**: Easy integration with custom tools and services

## Getting Started

For detailed installation instructions, please refer to our [Installation Guide](/installation/general).

To understand each component in depth:
- [Chat Interface Concepts](/concepts/chat/)
- [MCP Gateway Concepts](/concepts/mcp-gateway/)  
- [Admin Interface Concepts](/concepts/admin/)

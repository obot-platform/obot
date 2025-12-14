# Obot

**The Complete MCP Management Platform**

Obot is an open-source platform that solves the key challenges of adopting the Model Context Protocol (MCP) at enterprise scale. Built for organizations that need to securely discover, manage, and govern AI integrations across their infrastructure.

## The Challenge

MCP is the industry standard that connects AI applications to your systems and data. But enterprise adoption faces critical challenges:

- **Discovery**: How do users find the right MCP servers without a sprawling, ungoverned catalog?
- **Security**: How do you validate server security, enforce access controls, and maintain comprehensive audit trails?
- **Operations**: How do you automate authentication, scale infrastructure as usage grows, and roll out updates across environments?
- **Governance**: How do you intercept unsafe requests and enforce corporate policies before they reach your systems?

Obot solves these challenges with a comprehensive management platform.

## Getting Started

To quickly try out Obot's end-user experience, visit [https://chat.obot.ai](https://chat.obot.ai). Note that you won't experience the administrative features here. 

To run Obot yourself, launch it using Docker:

```bash
docker run -d --name obot -p 8080:8080 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -e OPENAI_API_KEY=<API KEY> \
  ghcr.io/obot-platform/obot:latest
```

Open your browser to [http://localhost:8080](http://localhost:8080) to access the Obot UI.

> **Note**: Replace `<API KEY>` with your [OpenAI API Key](https://platform.openai.com/api-keys). You can also set `ANTHROPIC_API_KEY` to use Anthropic models, or configure model providers through the Admin UI.

For more installation methods, see our [Installation Guide](https://docs.obot.ai/installation/general).

## Platform Features

Obot consists of four key pillars:

### MCP Hosting

Deploy and manage MCP servers without operational overhead:

- **Production-Ready Infrastructure** – Test locally with Docker or deploy to Kubernetes for enterprise-grade workload management, scheduling, and resource allocation
- **Flexible Deployment** – Run Node.js, Python, or containerized MCP servers in secure sandboxed environments
- **Multiple Server Types** – Run single-user STDIO servers or multi-user streamable HTTP servers
- **Role-Based Deployment** – Control who can deploy servers from the catalog, create custom servers, or share servers with others
- **OAuth 2.1 and Token Management** – Obot handles secure authentication and credential management so developers focus on server functionality

### MCP Registry

Centralized discovery and distribution of MCP servers:

- **Curated Catalog** – IT teams manage which MCP servers are available to users
- **Corporate Credentials** – Users access servers using existing authentication
- **MCP Registry Specification** – Conforms to the official standard for maximum client compatibility
- **Role-Based Discovery** – Users see only the servers they're authorized to access

### MCP Gateway

Secure, governed access to MCP servers:

- **Access Control Rules** – Define which users and groups can access specific servers
- **Usage Analytics** – Understand which servers are most valuable to your organization
- **Audit Trails** – Log all MCP server interactions for compliance and security review
- **Request Filtering** – Intercept and programmatically inspect MCP calls to enforce custom business logic and security policies

### Obot Chat

Production-ready chat interface with enterprise capabilities:

- **Multi-LLM Support** – Works with OpenAI, Anthropic, and other providers
- **Knowledge Integration** – Add domain-specific information to conversations with built-in RAG
- **Memory** – Maintain context across conversations for personalized interactions
- **Project Templates** – Create and share reusable project configurations across teams
- **Automated Tasks** – Schedule recurring tasks on a cron for workflow automation

## How It Works Together

1. **IT Administrators** curate MCP servers in the **Registry** and define access policies
1. **Security Teams** monitor usage, audit activity, and enforce compliance through the **Core Platform**
1. **Developers** build and deploy MCP servers using **Hosting** infrastructure
1. **Users** discover authorized servers through the **Gateway** and interact via **Chat** or third-party *MCP clients*

## Technical Highlights

- **Self-Hosted**: Deploy on your own infrastructure for complete control over data and security
- **MCP Standard**: Built on the open Model Context Protocol for maximum interoperability
- **Enterprise Security**: OAuth 2.1, encryption at rest and in transit, comprehensive audit logging
- **Extensible**: Easy integration with custom tools, services, and existing enterprise systems
- **GitOps Ready**: Manage catalog and configuration as code

## Documentation

For detailed documentation, visit [https://docs.obot.ai](https://docs.obot.ai)

## Community and Support

- **Documentation**: [https://docs.obot.ai](https://docs.obot.ai)
- **Discord**: [https://discord.com/invite/9sSf4UyAMC](https://discord.com/invite/9sSf4UyAMC)

## License

Obot is open-source software. See the LICENSE file for details.
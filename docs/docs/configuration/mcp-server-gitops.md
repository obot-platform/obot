---
title: MCP Server GitOps
---

## Overview

Obot supports managing MCP servers through Git repositories, enabling GitOps workflows. Instead of manually adding MCP servers one at a time, administrators can source server configurations from Git repositories. This supports collaborative workflows with proper code review, versioning, and automated validation processes.

### Key Benefits

- **Version Control**: Change tracking, rollback capabilities, and branch-based development
- **Collaborative Workflows**: PR-based reviews, team collaboration, and approval processes
- **Validation & Quality Assurance**: Automated testing, CI/CD integration, and consistent formatting
- **Automation**: Integration with existing DevOps workflows and automated deployment

## Getting Started

1. **Create or Fork a Repository**: Start with the official [Obot MCP server repository](https://github.com/obot-platform/mcp-catalog) or create your own
2. **Add Server Configurations**: Create YAML files for each MCP server following the format below
3. **Configure Obot**: Point your Obot instance to the Git repository containing your server configurations
4. **Establish Review Workflows**: Set up branch protection rules and PR-based review processes for configuration changes
5. **Automate Validation**: Implement CI/CD pipelines to validate YAML syntax and test server configurations

## Configuration Format

MCP server configurations consist of individual YAML files, each defining a single MCP server. These files contain comprehensive metadata including:

- **Name and Description**: Human-readable identification
- **Tool Previews**: Documentation of available tools and their parameters
- **Metadata**: Categories, icons, repository URLs, and classification information
- **Environment Variables**: Required and optional configuration parameters
- **Runtime Configuration**: Deployment and connection details

For examples and reference implementations, see the official Obot MCP server repository at [github.com/obot-platform/mcp-catalog](https://github.com/obot-platform/mcp-catalog).

## YAML Configuration Structure

Each MCP server is defined in its own YAML file with the following structure:

### Basic Information

```yaml
name: Server Name
description: |
  Detailed description of the server's capabilities and features.
  Supports multi-line markdown formatting.
```

### Tool Previews

```yaml
toolPreview:
  - name: tool_name
    description: Description of what this tool does
    params:
      param1: Parameter description
      param2: Optional parameter description (optional)
```

### Metadata and Classification

```yaml
metadata:
  categories: Category Name, Another Category
  unsupportedTools: tool1,tool2  # Optional
icon: https://example.com/icon.png
repoURL: https://github.com/owner/repo
```

### Environment Variables

```yaml
env:
  - key: ENVIRONMENT_VARIABLE
    name: Human Readable Name
    required: true
    sensitive: true
    description: Description of this variable
```

### Runtime Configuration

For remote servers:

```yaml
runtime: remote
remoteConfig:
  hostname: api.example.com
  fixedURL: https://api.example.com/mcp  # Alternative to hostname
  headers:
    - name: Authorization Header
      description: API token description
      key: Authorization
      required: true
      sensitive: true
```

For local packages:

```yaml
runtime: uvx
uvxConfig:
  package: 'package-name@latest'
```

## Complete Example

Here's a full example of an MCP server configuration file (`github.yaml`):

```yaml
name: GitHub
description: |
  A Model Context Protocol (MCP) server that provides easy connection to GitHub using the hosted version – no local setup or runtime required. Access comprehensive GitHub functionality through a remote server with additional tools not available in the local version.

  ## Features
  - **Repository Management**: Browse and query code, search files, analyze commits, and understand project structure
  - **Issue & PR Automation**: Create, update, and manage issues and pull requests with AI assistance
  - **CI/CD & Workflow Intelligence**: Monitor GitHub Actions workflow runs, analyze build failures, and manage releases
  - **Code Analysis**: Examine security findings, review Dependabot alerts, and get comprehensive codebase insights

  ## What you'll need to connect
  **Required:**
  - **Personal Access Token**: GitHub Personal Access Token with appropriate repository permissions

toolPreview:
  - name: create_issue
    description: Create a new issue in a GitHub repository
    params:
      owner: Repository owner
      repo: Repository name
      title: Issue title
      body: Issue body content (optional)
      labels: Labels to apply to this issue (optional)
  - name: create_pull_request
    description: Create a new pull request in a GitHub repository
    params:
      base: Branch to merge into
      head: Branch containing changes
      owner: Repository owner
      repo: Repository name
      title: PR title
      body: PR description (optional)

metadata:
  categories: Developer Tools
  unsupportedTools: create_or_update_file,push_files
icon: https://avatars.githubusercontent.com/u/9919?v=4
repoURL: https://github.com/github/github-mcp-server

runtime: remote
remoteConfig:
  hostname: api.githubcopilot.com
  headers:
  - name: Personal Access Token
    description: GitHub PAT
    key: Authorization
    required: true
    sensitive: true
```

This example demonstrates all the key components: descriptive content with markdown formatting, tool previews with parameter documentation, metadata classification, and remote runtime configuration with authentication headers.

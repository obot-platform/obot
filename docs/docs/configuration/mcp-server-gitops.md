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

## Adding a Git Source URL

Administrators can add Git repositories as catalog sources from the **Admin → MCP Servers → Git Source URLs** tab. Click **Add server(s) from Git** and enter the repository URL.

:::note

Connection URLs for MCP servers are derived from catalog entry names. Git-synced catalog entries have deterministic names based on the repository and file path, so the connection URL remains the same even if you remove and re-add the Git repository.

:::

### Supported URL formats

| Platform | Example |
|---|---|
| GitHub | `https://github.com/org/repo` or `https://github.com/org/repo.git` |
| GitHub with branch | `https://github.com/org/repo/my-branch` |
| GitLab | `https://gitlab.com/org/repo` or `https://gitlab.com/org/repo.git` |
| GitLab with branch | `https://gitlab.com/org/repo/my-branch` |
| GitLab with subgroups | `https://gitlab.com/group/subgroup/repo.git` |
| Self-hosted | `https://git.example.com/org/repo.git` |

For GitHub and GitLab a `.git` suffix is optional. For self-hosted instances it is required. To specify a branch on GitHub or GitLab, append it after the repo name (e.g. `/my-branch`). GitLab subgroup repositories require the `.git` suffix to distinguish the subgroup path from a branch name.

### Private repositories

To pull from a private repository, enter a **Personal access token** in the optional field below the URL. The token is stored securely and never returned by the API after saving.

**Required token scopes:**

- **GitHub**: `repo` (read access is sufficient)
- **GitLab**: `read_repository` (clone access) + `read_api` (pre-clone size check)

If no per-URL token is configured, Obot falls back to the `GITHUB_AUTH_TOKEN` environment variable.

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

### Server User Type

```yaml
serverUserType: singleUser  # Optional, defaults to "singleUser"
```

The `serverUserType` field specifies how users interact with the catalog entry:

- `singleUser` (default): Each user who installs this catalog entry gets their own independent MCP server instance.

Omitting the field or setting it to `""` is equivalent to `singleUser`.

### Kubernetes Secret Bindings

Secret bindings let you wire an env var, header, or file to a key in an externally-managed Kubernetes Secret instead of asking the user to supply the value at install time.

Secret bindings are only available on git-managed catalog entries, and only when Obot is using the Kubernetes MCP runtime backend.

#### Basic env var binding

The resolved value is injected into the MCP server pod as an environment variable — this works for `npx`, `uvx`, and `containerized` runtimes (not `remote`, which uses header bindings instead).

```yaml
env:
  - key: API_KEY
    name: API Key
    required: true
    sensitive: true
    description: Bound to a pre-existing Kubernetes Secret — no user input needed.
    secretBinding:
      name: my-secret       # Kubernetes Secret name
      key: api_key          # Key within that Secret
```

**Constraints:**
- Not supported for `remote` runtime env vars (use a header binding instead).
- `required: false` is allowed — when the Secret or key is absent the server deploys without that env var.
- For `remoteConfig.urlTemplate`, `${VAR}` placeholders must not reference env vars that use `secretBinding`.

#### File binding

When `file: true` the secret value is written to a file under `/files/` and the env var is set to the file path. This is useful for secrets that applications expect to read from the filesystem.

```yaml
env:
  - key: TLS_CERT
    name: TLS Certificate
    file: true
    required: true
    sensitive: true
    description: PEM certificate; mounted as a file at the path stored in TLS_CERT.
    secretBinding:
      name: my-tls-secret
      key: tls.crt
```

The application reads the certificate path from `os.Getenv("TLS_CERT")` and opens the file at that path.

#### Dynamic file binding

Adding `dynamicFile: true` (requires `file: true`) allows the mounted file to update **without restarting the pod** when the source Kubernetes Secret changes. The application is responsible for watching/re-reading the file for changes. This only has an effect when `file: true`.

```yaml
env:
  - key: API_CREDENTIALS
    name: API Credentials File
    file: true
    dynamicFile: true
    required: true
    sensitive: true
    description: Credentials file updated in-place when the Secret rotates — no pod restart needed.
    secretBinding:
      name: rotating-api-creds
      key: credentials.json
```

**Constraints:**
- `dynamicFile` is ignored unless `file: true`.
- `file` and `dynamicFile` are not supported on header bindings.

#### Header binding (remote servers)

For `remote` runtime servers, bind an outbound HTTP header to a Kubernetes Secret key:

```yaml
runtime: remote
remoteConfig:
  fixedURL: https://api.example.com/mcp
  headers:
    - key: Authorization
      name: API Token
      required: true
      sensitive: true
      secretBinding:
        name: api-token-secret
        key: token
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

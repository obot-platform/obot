# Obot's Open Source Registry
Each Obot instance connects to our open source registry by default. You can follow this guide to add your MCP server to [Obot's registry](https://github.com/obot-platform/mcp-catalog). You'll either submit **just a catalog entry**, or **a catalog entry + a repackaged image** (for containerized servers).

---

## Step 0 — Pick your path

**Question:** What type of MCP server are you shipping?

- **Remote HTTP (Hosted Service)**
    
    You expose a hosted HTTP MCP endpoint at a fixed URL or domain.
    
    → You only need [**mcp-catalog**](https://github.com/obot-platform/mcp-catalog).
    
- **Docker-based HTTP or Stdio npx/uvx**
    
    You have a Docker image that serves **HTTP MCP** **or** you have a **stdio** npx/uvx MCP server.
    
    → You'll touch **both** repos: first [**mcp-images**](https://github.com/obot-platform/mcp-images) (to repackage it), then [**mcp-catalog**](https://github.com/obot-platform/mcp-catalog).
    

**⚠️ Not Currently Supported:**
- Docker images that use stdio transport (only HTTP-based Docker images are supported)
- npx/uvx packages that serve streamable HTTP (only stdio-based npx/uvx packages are supported)

---

## Step 1 — Create your catalog entry

Fork from: `https://github.com/obot-platform/mcp-catalog`

Add a file: `your-server-name.yaml`

### Base Template (required for all servers)

```yaml
name: Your Server Name
description: |
  One-line summary of what this server does.

  *## Features*
  - Key capability 1 
  - Key capability 2 

  *## What you'll need to connect*
  - API key from https://example.com/api-keys
  - Account with "read" scope enabled
  - (Optional) Optional

metadata:
  categories: category-a, category-b

icon: https://example.com/icon.png
repoURL: https://github.com/your-org/your-mcp-repo

*# Environment variables your server needs (optional)*
env:
  - key: API_KEY                     *# Actual env var name*
    name: API Key                    *# Display name in UI*
    required: true                   *# Is this mandatory?*
    sensitive: true                  *# Masks value in UI (for credentials)*
    description: Your API key from the developer dashboard

*# Preview your server's tools (list top 20 if you have many)*
toolPreview:
  - name: search_documents
    description: Search through indexed documents using natural language
    params:
      query: Search query string
      limit: Maximum number of results (1-100)
  - name: get_document
    description: Retrieve a specific document by ID
    params:
      document_id: Unique identifier for the document
```

Now choose **exactly one** runtime block below.

<details>
  <summary>Option A: Remote HTTP Server</summary>

Choose the case that matches your setup:

### Case 1: Fixed Endpoint URL

Use this when your server has one static URL for all users:

```yaml
runtime: remote
remoteConfig:
  fixedURL: https://api.example.com/v1/mcp
  headers:
    - name: Personal Access Token
      description: PAT
      key: Authorization           # HTTP header name
      required: true
      sensitive: true

```

### Case 2: Same Hostname, User Selects Path

Use this when users connect to different paths on your domain:

```yaml
runtime: remote
remoteConfig:
  hostname: api.example.com
  headers:
    - name: API Key
      description: Your API key from <https://example.com/settings>
      key: X-API-Key
      required: true
      sensitive: true

```

Users will specify their path when connecting (e.g., https://api.example.com/serviceA/mcp).

### Case 3: URL Built from User Environment

Use this when the URL includes user-specific values:

```yaml
env:
  - key: WORKSPACE_URL
    name: Workspace URL
    description: "Your workspace URL, e.g., <https://mycompany.cloud.com>"
    required: true
    sensitive: false

runtime: remote
remoteConfig:
  URLTemplate: ${WORKSPACE_URL}/api/2.0/mcp/
  headers:
    - name: Personal Access Token
      description: PAT with workspace access
      key: Authorization
      required: true
      sensitive: true

```

---
</details>

<details>
  <summary>Option B: Containerized HTTP Server</summary>

**⚠️ Important:** If you have a Docker image that serves HTTP MCP, do NOT complete this section yet.
**Instead:**

1. Complete **Step 2** below to repackage your Docker image
2. Wait for your image to be published to GHCR
3. Return here and use this runtime block:

```yaml
runtime: containerized
containerizedConfig:
  image: ghcr.io/obot-platform/mcp-images/<your-server-name>:<tag>
  port: <port-number>           # Your container's exposed HTTP port
  path: /mcp           # HTTP path where MCP endpoint is served
  args:                # Optional runtime flags
    - flags # flags needed.

```

**Requirements:**

- Your container must serve HTTP/SSE on the specified port
- The MCP endpoint must be available at the specified path

---
</details>


<details>
  <summary>Option C: Stdio MCP Server (npx, uvx, docker)</summary>

**⚠️ Important:** If your server is stdio-based, do NOT complete this section yet.
**Instead:**

1. Complete **Step 2** below to repackage your stdio server
2. Wait for your image to be published to GHCR
3. Return here and use this runtime block:

```yaml
runtime: containerized
containerizedConfig:
  image: ghcr.io/obot-platform/mcp-images/<your-server-name>:<tag>
  port: 8099         # Fixed for stdio servers
  path: /            # Fixed for stdio servers
  args:
    - <your-mcp-server-command>           # Command to run your stdio server
    # Add any additional flags your server needs:
    # - --region
    # - us-east-1

```

The `args` must include the command that starts your stdio server, plus any required flags.

---
</details>

---

---

## Step 2 — Repackage and publish an image

If your server is containerized (Docker-based HTTP or stdio-based npx/uvx), we'll repackage and publish it.

Fork from: `https://github.com/obot-platform/mcp-images`

Edit `repackaging/images.yaml` and add your entry:

### Docker-based HTTP Servers

```yaml
- name: my-http-server
  type: docker
  package: ghcr.io/yourorg/mcp-http-server    # Your existing Docker image
  version: 1.0.0                              # Specific version tag

```

### Node.js (npx) Servers

```yaml
- name: my-awesome-server
  type: node
  package: @myorg/mcp-awesome-server    # NPM package name
  version: 1.2.3                        # Specific version

```

### Python (uvx) Servers

```yaml
- name: my-python-server
  type: python
  package: mcp-python-server            # PyPI package name
  version: 2.0.1

```

### After Your Image is Published

Once your PR in `mcp-images` merges, we'll publish:

```
ghcr.io/obot-platform/mcp-images/<your-server-name>:<version>

```

**Then** return to Step 1 and:
- For Docker-based HTTP servers: complete **Option B** with your published GHCR image
- For stdio servers: complete **Option C** with your published GHCR image

---

## Step 3 — Open your PR(s)

### For Remote HTTP Servers (Option A)

Open **one PR** in `mcp-catalog` adding `your-server-name.yaml`

### For Docker-based HTTP or Stdio Servers (Options B & C)

1. Open a PR in `mcp-images` editing `repackaging/images.yaml`
2. Wait for the image to be published to GHCR (typically within a few hours after the PR is merged)
3. Open a PR in `mcp-catalog` referencing the published GHCR image

---

## Pre-Submission Checklist

Before opening your PR, verify:

- [ ]  `description` includes **`## Features`** and **`## What you'll need to connect`** sections
- [ ]  `metadata.categories` lists a few categories
- [ ]  `icon` is a square image, publicly accessible
- [ ]  `repoURL` points to GitHub (or documentation if unavailable)
- [ ]  `toolPreview` lists your tool schemas (top 20 if you have many)
- [ ]  **Exactly one** runtime block (remote OR containerized)
- [ ]  All `env` variables have clear descriptions
- [ ]  Sensitive credentials are marked `sensitive: true`
- [ ]  (Docker-based HTTP or Stdio only) Image is published to GHCR before catalog PR

## Testing Your MCP Server Configuration

### Test with Obot running Locally

If you have Obot running locally, you can test your catalog entry before submitting:

1. When you run Obot locally, you'd need to set this env variable `OBOT_SERVER_DEFAULT_MCPCATALOG_PATH` to the absolute path of your local mcp-catalog fork.
2. Add your YAML to your local Obot catalog directory
3. On the MCP Servers page in the Admin UI, click **`sync`**
4. Attempt to connect to your server through the UI
5. Verify all tools appear and function correctly

---

## Examples:

Please refer to entries in https://github.com/obot-platform/mcp-catalog
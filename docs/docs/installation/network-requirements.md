---
title: Network requirements
---

# Network requirements

This page lists the outbound connections Obot makes to hosts outside your deployment, what each one is for, and how to
change or avoid it. Use it to write firewall or egress rules, or to plan an installation with restricted internet
access.

Connections inside your deployment, such as to PostgreSQL, the Kubernetes API, and MCP server pods, aren't listed.
With some cloud authentication methods, Obot also reaches the cloud instance metadata endpoint (`169.254.169.254`).

## Connections from the Obot server

The Obot server makes these connections on a default installation, whichever MCP runtime you use.

To allow all of them, allow outbound HTTPS to:

- `api.keygen.sh`
- `upgrade-server.obot.ai`
- `github.com`, `release-assets.githubusercontent.com`, and `objects.githubusercontent.com`
- `models.dev`
- `model-service.obot.ai`

### License validation

- **Destination:** `https://api.keygen.sh`
- **When:** At startup, every 24 hours, and when a key is added, changed, or rechecked. Only when a license key is
  configured.
- **If it fails:** Obot keeps the entitlements from the last successful check. See
  [How Obot validates a license](../enterprise/overview.md#how-obot-validates-a-license).
- **To change or avoid it:** Required to use a license key. Air-gapped installations can't validate a key yet.
  Support for offline license key validation is coming soon.
- **Future change:** A future release will validate licenses through a host on the `obot.ai` domain instead of
  `api.keygen.sh`.

### Update checks, registration, and product analytics

- **Destination:** `https://upgrade-server.obot.ai`
- **When:** Update checks every 24 hours. [Community registration](../enterprise/overview.md#registering-obot-community)
  when an administrator registers. [Product analytics](../configuration/product-analytics.md) once a day, with
  administrator consent, or always when `OBOT_SERVER_PRODUCT_ANALYTICS_MODE=on`.
- **If it fails:** Not fatal. Registration fails with an error.
- **To change or avoid it:** `OBOT_SERVER_DISABLE_UPDATE_CHECK=true` stops update checks, and
  `OBOT_SERVER_PRODUCT_ANALYTICS_MODE=off` stops analytics. `OBOT_UPGRADE_SERVER_URL` changes the host for all three.

### Default MCP catalogs

- **Destination:** `https://github.com` (`obot-platform/mcp-catalog` and `obot-platform/system-mcp-catalog`)
- **When:** At startup and every hour.
- **If it fails:** The catalog keeps its existing entries and shows a sync error.
- **To change or avoid it:** Set `OBOT_SERVER_DEFAULT_MCPCATALOG_PATH` and `OBOT_SERVER_DEFAULT_SYSTEM_MCPCATALOG_PATH`
  to a local directory or an internal Git mirror before first startup. They only take effect when the catalogs are first
  created. On a host other than GitHub, GitLab, or Bitbucket, the repository path in a mirror URL must end in `.git`,
  optionally followed by `/<branch>`. Otherwise Obot fetches it as a single file instead of cloning it. Without a
  branch, Obot uses `main`, but the default catalogs use the `v2-schema` branch, so include it, for example
  `https://git.example.com/obot/mcp-catalog.git/v2-schema`.

### Default skill repository

- **Destination:** `https://github.com` (`obot-platform/skills`)
- **When:** Every hour. The repository is created on first startup.
- **If it fails:** The repository shows a sync error.
- **To change or avoid it:** Set `OBOT_DEFAULT_SKILL_REPO_URL` to the HTTPS URL of an internal Git mirror before first
  startup, or delete the skill repository in the admin UI. On a host other than GitHub, GitLab, or Bitbucket, the URL
  must end in `.git`, for example `https://git.example.com/obot/skills.git`. A local path isn't supported, and Obot
  doesn't start if the value isn't an HTTPS Git URL.

### Obot Sentry MDM assets

- **Destination:** `https://github.com`, `https://release-assets.githubusercontent.com`, and
  `https://objects.githubusercontent.com` (the `obot-platform/obot-sentry` release tarball)
- **When:** When the source changes, and every hour until it succeeds.
- **If it fails:** Not fatal. [Obot Sentry](../device-management/how-sentry-works.md) MDM assets aren't available until
  it succeeds.
- **To change or avoid it:** Set `OBOT_SERVER_MDM_ASSET_SOURCE` to a local directory, a local tar archive, or an
  internal URL.

### Model pricing

- **Destination:** `https://models.dev`
- **When:** At startup and every hour.
- **If it fails:** Not fatal. LLM cost reporting uses the last synced data.
- **To change or avoid it:** Set `OBOT_SERVER_MODEL_INFO_SOURCE_URL` to an HTTP(S) URL for an internal mirror of
  `api.json`. A local path isn't supported. If the mirror resolves to a private IP address, Obot blocks it unless
  `OBOT_SERVER_DISALLOW_PRIVATE_IPMCP` is `false`, which also allows remote MCP servers on private IP addresses.

### MCP server tester model

- **Destination:** `https://model-service.obot.ai`
- **When:** When a user runs the [MCP server tester](../mcp-gateway/inspector.md), and when an admin opens
  **Platform > Settings** to show model service usage. Only when no model provider is configured, a valid license is
  present, and **Enable Model Proxy** is on under **Platform > Settings > Model Proxy**.
- **If it fails:** The tester can't generate, and **Platform > Settings** shows model service usage as N/A.
- **To change or avoid it:** Configure a model provider, turn off **Enable Model Proxy**, or set
  `OBOT_SERVER_MODEL_PROXY_URL` to an empty string to turn it off.

Startup doesn't depend on any of these. Obot starts without internet access, and the affected features recover once
the destination is reachable.

## Connections for MCP servers

Where MCP server traffic comes from depends on the MCP runtime: the Docker host when Obot runs MCP servers with
Docker, or the cluster's nodes and pods when it runs them on Kubernetes. Both runtimes need:

| Destination | Purpose | When | How to change or avoid it |
|---|---|---|---|
| `ghcr.io` | MCP server base images (`ghcr.io/obot-platform/mcp-images/*`) | Each time an MCP server starts | Mirror the images and set `OBOT_SERVER_MCPBASE_IMAGE`, `OBOT_SERVER_MCPHTTPWEBHOOK_BASE_IMAGE`, and `OBOT_SERVER_MCPOPEN_APIIMAGE`. See [Server configuration](../configuration/server-configuration.md). |
| The registry of each containerized MCP server's image | The MCP server's image | Each time the MCP server starts | Use images from a registry you can reach |
| `registry.npmjs.org`, `pypi.org`, `files.pythonhosted.org` | Packages for `npx` and `uvx` MCP servers, downloaded by the MCP server container | Each time one of those MCP servers starts | Set a registry env var on each `npx` or `uvx` server, for example a static `NPM_CONFIG_REGISTRY` or `UV_INDEX_URL` value. Obot has no global setting for this. Or use containerized MCP servers with images in your registry. |
| Whatever each MCP server calls | The MCP server's own work, such as a SaaS API | While the MCP server runs | Depends on the MCP server |

If an MCP server's image or packages can't be downloaded, that MCP server doesn't start. Other MCP servers and the rest
of Obot keep working.

### Docker runtime

- The Docker daemon pulls the images, so it needs registry access and uses its own proxy configuration, not Obot's.
  Obot asks for a pull each time an MCP server starts, and falls back to a local copy only when the registry refuses
  the pull, not when it can't be reached.
- Obot also uses `alpine:latest` from `docker.io` for short-lived setup containers. It's needed for every `npx` and
  `uvx` server and for any MCP server with file configuration, and is pulled only if the Docker host doesn't already
  have it. The image can't be changed, so pre-pull it on hosts without access to `docker.io`.
- MCP server containers reach the internet through the Docker host's network.

### Kubernetes runtime

- The kubelet on each node pulls the images, with `imagePullPolicy: Always`, so nodes need registry access each time
  an MCP server pod starts. For private registries, use
  [image pull secrets](../configuration/mcp-deployments-in-kubernetes.md#image-pull-secrets).
- MCP server pods reach the internet through the cluster's network. Restrict it with the
  [network policy for MCP servers](./kubernetes-deployment.md#network-policy-for-mcp-servers) or
  [MCP server egress control](../configuration/mcp-server-egress-control.md).
- The Obot container image, and the images for any other part of your deployment, are pulled by the cluster from
  `ghcr.io`, not by Obot.

## Connections that depend on your configuration

These connections only happen when you configure the feature, and go to the destination you configure.

| Feature | Destination |
|---|---|
| [Auth providers](../configuration/auth-providers.md) | Your identity provider: GitHub, Google, Entra, Okta, JumpCloud, or Auth0 |
| [Model providers](../configuration/model-providers.md) and the LLM gateway | The configured model API, for example `api.openai.com`, `api.anthropic.com`, Azure OpenAI and Azure AI Foundry endpoints, or `bedrock-mantle.<region>.api.aws`. Message policies use the same providers. |
| Remote MCP servers | The server's URL, plus its OAuth authorization server for discovery, dynamic client registration, and token exchange |
| MCP catalogs and skill repositories added by an admin | The repository host over HTTPS, usually `github.com`, `gitlab.com`, or `bitbucket.org`. For GitHub and GitLab, Obot also calls `api.github.com` or `gitlab.com/api/v4` to check the repository size before cloning. |
| OAuth clients connecting to Obot | The client's Client ID Metadata Document URL and `jwks_uri`, when the client uses them |
| [Encryption providers](../configuration/encryption-providers/overview.md) | AWS KMS, Google Cloud KMS, or Azure Key Vault. Obot doesn't start if the provider can't be reached. |
| Audit log storage and exports | Amazon S3 or an S3-compatible endpoint, Google Cloud Storage, or Azure Blob Storage |
| Image pull secrets for Amazon ECR (Kubernetes only) | AWS STS and ECR, about an hour before each token expires |
| MCP server egress control (Kubernetes only) | The Helm repository in `OBOT_SERVER_MCPNETWORK_POLICY_PROVIDER_CHART_REPO`, at startup. Obot doesn't start if it can't fetch the chart. Set `OBOT_SERVER_MCPNETWORK_POLICY_PROVIDER_CHART_PATH` to use a local chart instead. |
| [Claude Environments](../claude-environments/overview.md) | Anthropic, your container registry, and your snapshot storage. See [Cluster requirements](../claude-environments/cluster-requirements.md). |

## Connections from users' browsers

The Obot UI loads these from the user's browser, not from the Obot server:

- Fonts from `fonts.googleapis.com` and `fonts.gstatic.com`
- Code highlighting styles from `cdnjs.cloudflare.com`
- MCP server icons from the URLs in the catalog
- Embedded videos from YouTube in in-app guides

The UI works without them, with fallback fonts and missing icons.

## Using an HTTP proxy

The Obot server honors the standard `HTTPS_PROXY`, `HTTP_PROXY`, and `NO_PROXY` environment variables for its own
requests, including license validation, update checks, catalog and skill syncs, model pricing, the LLM gateway's
OpenAI, Anthropic, Azure, and Bedrock calls, and remote MCP servers. The encryption providers and Claude Environments
also receive them.

Some connections don't go through the proxy:

- **Model and auth providers** run as separate processes that don't inherit the proxy variables.
- **MCP servers** don't receive the proxy variables. Set them in each MCP server's environment if they need them.
- **Image pulls** use your container runtime's proxy configuration, not Obot's.

Proxy support for model providers, auth providers, and MCP servers is coming soon.

Remote MCP servers, MCP OAuth, model pricing, OpenAPI imports, and catalogs fetched from a plain URL go through Obot's
protection against requests to local and private networks. For those connections:

- Obot resolves the destination hostname with its own DNS before sending the request through the proxy, so it needs
  DNS resolution for external hosts.
- A proxy on a private IP address is blocked unless `OBOT_SERVER_DISALLOW_PRIVATE_IPMCP` is `false`, and a proxy on
  localhost is blocked unless `OBOT_SERVER_DISALLOW_LOCALHOST_MCP` is `false`. Either setting also allows remote MCP
  servers on those addresses.

License validation, update checks, and Git clones don't have these restrictions.

## Restricted and air-gapped installations

To run Obot without internet access:

1. Before first startup, point `OBOT_SERVER_DEFAULT_MCPCATALOG_PATH` and `OBOT_SERVER_DEFAULT_SYSTEM_MCPCATALOG_PATH`
   at local copies or internal Git mirrors of the catalogs, and `OBOT_DEFAULT_SKILL_REPO_URL` at the HTTPS URL of an
   internal Git mirror of the skills repository. Mirror repository paths must end in `.git`, followed by the
   `v2-schema` branch for the catalogs.
2. Point `OBOT_SERVER_MDM_ASSET_SOURCE` at a local copy or an internal URL, and `OBOT_SERVER_MODEL_INFO_SOURCE_URL` at
   an internal HTTP(S) mirror. See [Model pricing](#model-pricing) for mirrors on private IP addresses.
3. Set `OBOT_SERVER_DISABLE_UPDATE_CHECK=true`, `OBOT_SERVER_PRODUCT_ANALYTICS_MODE=off`, and
   `OBOT_SERVER_MODEL_PROXY_URL=""`.
4. Mirror the MCP base images and set the image variables. With the Docker runtime, also pre-pull `alpine:latest` on
   the Docker host.
5. Use containerized MCP servers with images in your registry, or set a registry env var, such as
   `NPM_CONFIG_REGISTRY` or `UV_INDEX_URL`, on each `npx` and `uvx` server.
6. Use an auth provider and model provider that you can reach.

Setting a URL variable to an empty string doesn't turn the feature off, except for `OBOT_SERVER_MODEL_PROXY_URL`: Obot
ignores empty values and uses the default. Point the variable at an internal mirror instead, or at a local path where
that setting supports one.

A license key can't be validated without access to `https://api.keygen.sh`, so an air-gapped installation runs as
unregistered Obot Community for now. Support for offline license key validation for air-gapped installations is
coming soon. See [Editions and licensing](../enterprise/overview.md#when-the-license-service-cant-be-reached).

---
title: Network requirements
---

# Network requirements

This page lists the outbound connections Obot makes to hosts outside your deployment, what each one is for, and how to
change or avoid it. Use it to write firewall or egress rules, or to plan an installation with restricted internet
access.

Connections inside your deployment, such as to PostgreSQL, the Kubernetes API, and MCP server pods, aren't listed.
With some cloud authentication methods, Obot also reaches the cloud instance metadata endpoint (`169.254.169.254`).

## Connections Obot makes by default

These connections happen on a default installation without any configuration.

| Destination | Purpose | When | If it fails | How to change or avoid it |
|---|---|---|---|---|
| `https://api.keygen.sh` | [License validation](../enterprise/overview.md#how-obot-validates-a-license) | At startup, every 24 hours, and when a key is added, changed, or rechecked. Only when a license key is configured. | Obot keeps the entitlements from the last successful check | Required to use a license key |
| `https://upgrade-server.obot.ai` | Update checks, [Community registration](../enterprise/overview.md#registering-obot-community), and [product analytics](../configuration/product-analytics.md) | Update checks every 24 hours. Registration when an admin registers. Analytics once a day, only with consent. | Not fatal. Registration fails with an error. | `OBOT_SERVER_DISABLE_UPDATE_CHECK=true` stops update checks. `OBOT_SERVER_PRODUCT_ANALYTICS_MODE=off` stops analytics. `OBOT_UPGRADE_SERVER_URL` changes the host for all three. |
| `https://github.com` | Default MCP catalogs (`obot-platform/mcp-catalog`, `obot-platform/system-mcp-catalog`) | At startup and every hour | The catalog keeps its existing entries and shows a sync error | Set `OBOT_SERVER_DEFAULT_MCPCATALOG_PATH` and `OBOT_SERVER_DEFAULT_SYSTEM_MCPCATALOG_PATH` to a local directory or an internal mirror before first startup. They only take effect when the catalogs are first created. |
| `https://github.com` | Default skill repository (`obot-platform/skills`) | Every hour. Created on first startup. | The repository shows a sync error | Set `OBOT_DEFAULT_SKILL_REPO_URL` to an internal mirror before first startup, or delete the skill repository in the admin UI |
| `https://github.com`, `https://release-assets.githubusercontent.com`, `https://objects.githubusercontent.com` | [Obot Sentry](../device-management/how-sentry-works.md) MDM assets (`obot-platform/obot-sentry` release tarball) | When the source changes, and every hour until it succeeds | Not fatal | Set `OBOT_SERVER_MDM_ASSET_SOURCE` to a local directory, a local tar archive, or an internal URL |
| `https://models.dev` | Model pricing data for LLM cost reporting | At startup and every hour | Not fatal. Costs use the last synced data. | Set `OBOT_SERVER_MODEL_INFO_SOURCE_URL` to an internal mirror of `api.json` |
| `https://model-service.obot.ai` | Built-in model for the [MCP server tester](../mcp-gateway/inspector.md), used when no model provider is configured. Requires a valid license. | When a user runs the tester | The tester can't generate | Set `OBOT_SERVER_MODEL_PROXY_URL` to an empty string to turn it off |
| `ghcr.io` | MCP server base images (`ghcr.io/obot-platform/mcp-images/*`) | Each time an MCP server starts. Kubernetes pulls them with `imagePullPolicy: Always`. | The MCP server doesn't start | Mirror the images and set `OBOT_SERVER_MCPBASE_IMAGE`, `OBOT_SERVER_MCPHTTPWEBHOOK_BASE_IMAGE`, and `OBOT_SERVER_MCPOPEN_APIIMAGE`. See [Server configuration](../configuration/server-configuration.md). |
| `docker.io` | `alpine:latest`, used for init containers by the Docker MCP runtime | When an MCP server starts on Docker and the image isn't already present | The MCP server doesn't start | Pre-pull `alpine:latest` on the Docker host. It can't be changed. |
| `registry.npmjs.org`, `pypi.org`, `files.pythonhosted.org` | Packages for `npx` and `uvx` MCP servers | Each time one of those MCP servers starts | The MCP server doesn't start | Configure an internal registry through the MCP server's environment, for example `NPM_CONFIG_REGISTRY` or `UV_INDEX_URL`, or use containerized MCP servers |

Startup doesn't depend on any of these. Obot starts without internet access, and the affected features recover once
the destination is reachable.

The Obot container image itself, and the images for any [cloud deployment](./kubernetes-deployment.md), come from
`ghcr.io` and are pulled by your container runtime rather than by Obot.

## Connections that depend on your configuration

These connections only happen when you configure the feature, and go to the destination you configure.

| Feature | Destination |
|---|---|
| [Auth providers](../configuration/auth-providers.md) | Your identity provider: GitHub, Google, Entra, Okta, JumpCloud, or Auth0 |
| [Model providers](../configuration/model-providers.md) and the LLM gateway | The configured model API, for example `api.openai.com`, `api.anthropic.com`, Azure OpenAI and Azure AI Foundry endpoints, or `bedrock-mantle.<region>.api.aws`. Message policies use the same providers. |
| Remote MCP servers | The server's URL, plus its OAuth authorization server for discovery, dynamic client registration, and token exchange |
| Containerized MCP servers | The image's registry |
| Anything the MCP servers themselves call | Whatever the MCP server is built to reach. Obot doesn't proxy these. See [MCP server egress control](../configuration/mcp-server-egress-control.md). |
| MCP catalogs and skill repositories added by an admin | The repository host over HTTPS, usually `github.com`, `gitlab.com`, or `bitbucket.org`. For GitHub and GitLab, Obot also calls `api.github.com` or `gitlab.com/api/v4` to check the repository size before cloning. |
| OAuth clients connecting to Obot | The client's Client ID Metadata Document URL and `jwks_uri`, when the client uses them |
| [Encryption providers](../configuration/encryption-providers/overview.md) | AWS KMS, Google Cloud KMS, or Azure Key Vault. Obot doesn't start if the provider can't be reached. |
| Audit log storage and exports | Amazon S3 or an S3-compatible endpoint, Google Cloud Storage, or Azure Blob Storage |
| Image pull secrets for Amazon ECR | AWS STS and ECR, about an hour before each token expires |
| MCP server egress control | The Helm repository in `OBOT_SERVER_MCPNETWORK_POLICY_PROVIDER_CHART_REPO`, at startup. Obot doesn't start if it can't fetch the chart. Set `OBOT_SERVER_MCPNETWORK_POLICY_PROVIDER_CHART_PATH` to use a local chart instead. |
| [Claude Environments](../claude-environments/overview.md) | Anthropic, your container registry, and your snapshot storage. See [Cluster requirements](../claude-environments/cluster-requirements.md). |
| OpenTelemetry | The endpoint in `OTEL_EXPORTER_OTLP_ENDPOINT` |

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

Obot resolves the destination hostname with its own DNS before sending a request through the proxy, so it needs DNS
resolution for external hosts. If your proxy has a private IP address, Obot's protection against requests to private
networks blocks connections to remote MCP servers through it unless `OBOT_SERVER_DISALLOW_PRIVATE_IPMCP` is `false`.

## Restricted and air-gapped installations

To run Obot without internet access:

1. Before first startup, point `OBOT_SERVER_DEFAULT_MCPCATALOG_PATH` and `OBOT_SERVER_DEFAULT_SYSTEM_MCPCATALOG_PATH`
   at local copies or internal mirrors of the catalogs, and `OBOT_DEFAULT_SKILL_REPO_URL` at an internal mirror of the
   skills repository.
2. Point `OBOT_SERVER_MDM_ASSET_SOURCE` and `OBOT_SERVER_MODEL_INFO_SOURCE_URL` at local copies or internal mirrors.
3. Set `OBOT_SERVER_DISABLE_UPDATE_CHECK=true`, `OBOT_SERVER_PRODUCT_ANALYTICS_MODE=off`, and
   `OBOT_SERVER_MODEL_PROXY_URL=""`.
4. Mirror the MCP base images and set the image variables, and pre-pull `alpine:latest` if you use the Docker runtime.
5. Use containerized MCP servers with images in your registry, or configure `npx` and `uvx` servers to use internal
   package registries.
6. Use an auth provider and model provider that you can reach.

Setting a URL variable to an empty string doesn't turn the feature off, except for `OBOT_SERVER_MODEL_PROXY_URL`: Obot
ignores empty values and uses the default. Point the variable at a local path or internal mirror instead.

A license key can't be validated without access to `https://api.keygen.sh`, so an air-gapped installation runs as
unregistered Obot Community. See [Editions and licensing](../enterprise/overview.md#when-the-license-service-cant-be-reached).

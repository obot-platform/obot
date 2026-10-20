---
description: Environment variables for configuring the Obot server.
title: "Server configuration"
hide_table_of_contents: true
---

# Server configuration {#obot-configuration-reference}

The Obot server is configured via environment variables. The following configuration is available:

| Environment Variable | Description | Default |
|---------------------|-------------|---------|
| `OPENAI_API_KEY` | Configures the [LLM gateway](./model-providers.md#openai-and-anthropic-keys-at-startup)'s `openai` provider with this key at startup. If the value changes, Obot updates the provider's key on the next start. You can also add providers and keys under **LLM Gateway** → **Providers**. | - |
| `ANTHROPIC_API_KEY` | Configures the [LLM gateway](./model-providers.md#openai-and-anthropic-keys-at-startup)'s `anthropic` provider with this key at startup. If the value changes, Obot updates the provider's key on the next start. | - |
| `GITHUB_AUTH_TOKEN` | Obot makes heavy use of repositories hosted on GitHub. Care is taken to cache these and only re-check when necessary. However, rate-limiting can happen. Setting a read-only token here can alleviate many of these issues. No grants are required for either a 'classic' or 'fine-grained' token to access public repos (read-only). If you want to give the token access to private repos, you will need to give it `repo` (for a 'classic' token) or `contents (read-only)` and `metadata (read-only)`. |
| `OBOT_SERVER_GIT_MAX_REPO_SIZE_MB` | Maximum repository size for git clones, in MB (1 MB = 1,048,576 bytes). Applies to GitHub/GitLab size pre-checks and cumulative bytes written during cloning. Must be a positive integer; invalid values prevent server startup. Also available as `--git-max-repo-size-mb`. | `100` |
| `OBOT_SERVER_DSN` | Obot uses a database backend. By default, it will use a sqlite3 local database when running the plan Obot binary. The Obot container will use an internal PostgreSQL database (not recommended for production). This environment variable allows you to specify another database option. For example, you can use a postgres database with something like `OBOT_SERVER_DSN=postgres://user:password@host/database`. | - |
| `OBOT_SERVER_HOSTNAME` | Tell Obot what its server URL is so that things like OAuth, LLM proxying, and invoke URLs are handled correctly. | - |
| `OBOT_SERVER_DAILY_USER_INPUT_TOKEN_LIMIT` | Daily input token limit per user in the LLM gateway's *Daily token limit* budget, which the gateway puts on every profile when it first starts. Administrators are exempt. It is used only then; after that, edit each profile's budget on the **Profiles** tab of the **LLM Gateway** page. Set to a negative value to leave out this limit. | `10000000` |
| `OBOT_SERVER_DAILY_USER_OUTPUT_TOKEN_LIMIT` | Daily output token limit per user in the LLM gateway's *Daily token limit* budget, which the gateway puts on every profile when it first starts. Administrators are exempt. It is used only then. Set to a negative value to leave out this limit. | `100000` |
| `OBOT_SERVER_SINGLE_USER_IDLE_SERVER_SHUTDOWN_HOURS` | The interval in hours to check for idle single-user MCP servers and shut them down. Set to `-1` to disable idle shutdown. | `24` (1 day) |
| `OBOT_SERVER_MULTI_USER_IDLE_SERVER_SHUTDOWN_HOURS` | The interval in hours to check for idle multi-user MCP servers and shut them down. Set to `-1` to disable idle shutdown. | `168` (7 days) |
| `NAH_THREADINESS` | Sets the number of concurrent threads that can run in the Obot controller. | `10` |
| `KINM_DB_CONNECTIONS` | Sets both the maximum open and idle connection counts in the Kinm database pool. `KINM_DB_MAX_CONNECTIONS` and `KINM_DB_MAX_IDLE_CONNECTIONS` override the corresponding values. | `5` |
| `KINM_DB_MAX_IDLE_CONNECTIONS` | The maximum number of idle connections in the Kinm database pool. Overrides the idle connection count set by `KINM_DB_CONNECTIONS`. | `2` |
| `KINM_DB_MAX_CONNECTIONS` | The maximum number of open connections in the Kinm database pool. Overrides the open connection count set by `KINM_DB_CONNECTIONS`. | `5` |
| `KINM_DB_MAX_CONNECTION_LIFETIME_SECONDS` | The maximum lifetime of a connection in the Kinm database pool, in seconds. | `180` |
| `KINM_DB_WATCH_POLL_SECONDS` | How long a Kinm watch waits before listing again when nothing has woken it, in seconds. Only applies while the change listener is connected. Without a connected listener a watch polls every 2 seconds, which this does not change. | `120` |
| `KINM_DB_NOTIFY_DEBOUNCE_MILLISECONDS` | How often change notifications may wake the watches on one table, in milliseconds. The first change after a quiet moment is never delayed. | `1000` |
| `KINM_DB_DISABLE_NOTIFY` | Disables Kinm's use of PostgreSQL LISTEN/NOTIFY, so replicas learn about each other's writes by polling every 2 seconds instead. | `false` |
| `OBOT_AUTH_PROVIDER_POSTGRES_MAX_IDLE_CONNECTIONS` | The maximum number of idle connections in the PostgreSQL database pool used by authentication providers. | `2` |
| `OBOT_AUTH_PROVIDER_POSTGRES_MAX_CONNECTIONS` | The maximum number of open connections in the PostgreSQL database pool used by authentication providers. | `5` |
| `OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_LIFETIME_SECONDS` | The maximum lifetime of a connection in the PostgreSQL database pool used by authentication providers, in seconds. | `180` |
| `OBOT_SERVER_ENABLE_AUTHENTICATION` | Enables authentication for Obot | `false` |
| `OBOT_SERVER_UNAUTHENTICATED_RATE_LIMIT` | Rate limit for unauthenticated requests (requests per second). Unauthenticated requests are tracked by source IP address. | `100` |
| `OBOT_SERVER_AUTHENTICATED_RATE_LIMIT` | Rate limit for authenticated non-admin requests (requests per second). Authenticated requests are tracked by user ID. Admin users are exempt from rate limiting. | `200` |
| `OBOT_SERVER_ENCRYPTION_PROVIDER` | Configures an encryption provider for credentials in Obot. One of aws, gcp, azure, custom, or none | `none` |
| `OBOT_SERVER_ENCRYPTION_CONFIG_FILE` | The path to a file containing the encryption configuration. Only used when `OBOT_SERVER_ENCRYPTION_PROVIDER` is `custom` | - |
| `OBOT_SERVER_ENCRYPTION_KEY` | Sets the key to be used for encryption. Should only be set if `OBOT_SERVER_ENCRYPTION_PROVIDER` is `custom` | - |
| `OBOT_BOOTSTRAP_TOKEN` | Sets a bootstrap token. If authentication is enabled, one will be autogenerated for you if this is not set. | - |
| `OBOT_SERVER_LOCAL_AUTH_INITIAL_OWNER_EMAIL` | Provisions this email as the initial owner with the built-in Local provider. Must be set with `OBOT_SERVER_LOCAL_AUTH_INITIAL_OWNER_SETUP_TOKEN`. This disables bootstrap-token login. | - |
| `OBOT_SERVER_LOCAL_AUTH_INITIAL_OWNER_SETUP_TOKEN` | Randomly generated, high-entropy secret (at least 32 characters; for example, `openssl rand -hex 32`) bound to the initial owner setup link. Obot stores only its hash and revokes it when password setup completes. | - |
| `OBOT_SERVER_LOCAL_AUTH_INITIAL_OWNER_SETUP_TOKEN_EXPIRATION_HOURS` | Number of hours the initial owner setup link remains valid. | `168` |
| `OBOT_SERVER_AUTH_OWNER_EMAILS` | A comma separated list of email addresses that will have the Owner role in Obot. Email matching is case-insensitive. | - |
| `OBOT_SERVER_AUTH_ADMIN_EMAILS` | A comma separated list of email addresses that will have the Admin role in Obot. Email matching is case-insensitive. | - |
| `OBOT_SERVER_MCPAUDIT_LOG_RETENTION_DAYS` | The number of days to retain MCP audit logs before they are automatically deleted. Set to `0` to disable automatic cleanup. Use the [audit log export](./audit-log-export.md) functionality to preserve logs beyond this period. | `90` |
| `OBOT_SERVER_MCPAUDIT_LOG_PERSIST_INTERVAL_SECONDS` | The interval in seconds at which buffered MCP audit logs are flushed to the database. | `5` |
| `OBOT_SERVER_MCPAUDIT_LOGS_PERSIST_BATCH_SIZE` | The number of MCP audit log entries written to the database in a single batch. | `1000` |
| `OBOT_SERVER_LLMAUDIT_LOG_RETENTION_DAYS` | The number of days to retain LLM audit logs before they are automatically deleted. Set to `0` to disable automatic cleanup. | `90` |
| `OBOT_SERVER_MODEL_PROXY_URL` | MCP Tester model proxy service base URL, used for generation only when the LLM gateway has no providers configured. An explicitly empty string disables generation. Custom URLs must use HTTPS; HTTP is allowed only with `OBOT_DEV_MODE=true`. | `https://model-service.obot.ai` |
| `OBOT_SERVER_DISABLE_LLMAUDIT_LOG` | Disables collection and persistence of new LLM gateway audit logs. Existing logs remain available. | `false` |
| `OBOT_SERVER_DEVICE_SCAN_RETENTION_DAYS` | The number of days to retain submitted device scans before they are automatically deleted. Set to `0` to disable automatic cleanup. | `90` |
| `OBOT_SERVER_DEFAULT_MCPCATALOG_PATH` | The path to the default MCP catalog (accessible to all users). | - |
| `OBOT_SERVER_DEFAULT_SYSTEM_MCPCATALOG_PATH` | The path to the default System MCP catalog. | - |
| `OBOT_SERVER_MDM_ASSET_SOURCE` | The source for MDM assets. Can be a local directory, a tar archive path, or an HTTP(S) tarball URL. | `https://github.com/obot-platform/obot-sentry/releases/download/v0.1.8/mdm-assets.tar.gz` |
| `OBOT_SERVER_AUDIT_LOGS_MODE` | Configures the storage backend for audit logs in Obot. Can be 'off', 'disk', or 's3' | `off` |
| `OBOT_SERVER_AUDIT_LOGS_STORE_S3BUCKET` | The name of the S3 bucket to store audit logs in. | - |
| `OBOT_SERVER_AUDIT_LOGS_STORE_S3ENDPOINT` | If config.OBOT_SERVER_AUDIT_LOGS_MODE is 's3' and you are not using AWS S3, this needs to be set to the S3 api endpoint of your provider. | - |
| `OBOT_SERVER_AUDIT_LOGS_COMPRESS_FILE` | Controls whether or not to compress audit log files | `true` |
| `OBOT_SERVER_AUDIT_LOGS_STORE_USE_PATH_STYLE` | Whether to use path style for S3 object names | - |
| `OBOT_SERVER_MCPBASE_IMAGE` | Deploy MCP servers in the kubernetes cluster or using docker with this base image. | `ghcr.io/obot-platform/mcp-images/stdio-wrapper:v0.26.2` |
| `OBOT_SERVER_MCPOPEN_APIIMAGE` | Container image used to host OpenAPI MCP servers with the Docker or Kubernetes runtime backend. | `ghcr.io/obot-platform/mcp-images/openapi-mcp:v0.27.2` |
| `OBOT_SERVER_MCPIMAGE_PULL_SECRETS` | Comma-separated Kubernetes secret names to use as static image pull secrets for every MCP server Deployment. When set, managed image pull secrets are disabled. See [Image Pull Secrets](./image-pull-secrets.md). | - |
| `OBOT_SERVER_MCPSECRET_BINDING_ALLOWED_LABEL` | Kubernetes Secret label key required before a Secret can be selected or resolved by MCP secret bindings. Only label presence is checked; the label value is ignored. Secrets without this label are not shown as bindable targets in the admin UI and are treated as unavailable when Obot resolves a secret binding at runtime. | `obot.obot.ai/allow-secret-binding` |
| `OBOT_SERVER_MCPHTTPWEBHOOK_BASE_IMAGE` | Deploy MCP HTTP webhook servers in the cluster using this base image. | `ghcr.io/obot-platform/mcp-images/http-webhook-mcp-converter:v0.26.2` |
| `OBOT_SERVER_MCPRUNTIME_BACKEND` | The runtime backend to use for running MCP servers: docker or kubernetes. | `kubernetes` in the helm chart, `docker` otherwise |
| `OBOT_SERVER_MCPCLUSTER_DOMAIN` | The cluster domain to use for MCP services. Only matters if `OBOT_SERVER_MCPBASE_IMAGE` is set. | `cluster.local` |
| `OBOT_SERVER_SERVICE_NAME` | The Kubernetes service name for the obot server. Automatically set by the helm chart when using kubernetes backend. Used to construct the internal service FQDN for MCP runtime callbacks. | - |
| `OBOT_SERVER_SERVICE_NAMESPACE` | The Kubernetes namespace where the obot server runs. Automatically set by the helm chart when using kubernetes backend. Used to construct the internal service FQDN for MCP runtime callbacks. | - |
| `OBOT_SERVER_DISALLOW_LOCALHOST_MCP` | Disallow MCP servers that try to connect to localhost. Set to `false` only when you intentionally need MCP servers to reach localhost from the Obot runtime environment. | `true` |
| `OBOT_SERVER_DISALLOW_PRIVATE_IPMCP` | Disallow MCP servers that resolve to private IP addresses. Set to `false` only when you intentionally need MCP servers to reach private network services from the Obot runtime environment. | `true` |
| `OBOT_SERVER_DISALLOW_LINK_LOCAL_MCP` | Disallow MCP servers that resolve to link-local addresses. Set to `false` only when you intentionally need MCP servers to reach link-local services from the Obot runtime environment. | `true` |
| `OBOT_SERVER_MCPNETWORK_POLICY_PROVIDER_CHART_REPO` | Helm repository URL for the MCP server egress control provider chart. Used with `OBOT_SERVER_MCPNETWORK_POLICY_PROVIDER_CHART_NAME`. | - |
| `OBOT_SERVER_MCPNETWORK_POLICY_PROVIDER_CHART_NAME` | Helm chart name for the MCP server egress control provider. Setting this enables MCP server egress control. | - |
| `OBOT_SERVER_MCPNETWORK_POLICY_PROVIDER_CHART_VERSION` | Helm chart version for the MCP server egress control provider. | - |
| `OBOT_SERVER_MCPNETWORK_POLICY_PROVIDER_CHART_PATH` | Local filesystem path to an MCP server egress control provider chart. Setting this enables MCP server egress control and cannot be combined with chart repo, name, or version. | - |
| `OBOT_SERVER_MCPNETWORK_POLICY_PROVIDER_VALUES` | YAML or JSON values blob merged into the MCP server egress control provider chart values. | - |
| `OBOT_SERVER_MCPDEFAULT_DENY_ALL_EGRESS` | Default new MCP servers to deny all egress when MCP server egress control is enabled and no egress domains are configured. | `false` |
| `OBOT_SERVER_MCPPOD_SECURITY_ENABLED` | Enable Pod Security Admission labels on the MCP namespace. Only applies when using kubernetes backend. | `true` |
| `OBOT_SERVER_MCPPOD_SECURITY_ENFORCE` | Pod Security Standards level to enforce for MCP namespace (privileged, baseline, or restricted). Only applies when using kubernetes backend. | `restricted` |
| `OBOT_SERVER_MCPPOD_SECURITY_ENFORCE_VERSION` | Kubernetes version for the PSA enforce policy. Only applies when using kubernetes backend. | `latest` |
| `OBOT_SERVER_MCPPOD_SECURITY_AUDIT` | Pod Security Standards level to audit for MCP namespace (privileged, baseline, or restricted). Only applies when using kubernetes backend. | `restricted` |
| `OBOT_SERVER_MCPPOD_SECURITY_AUDIT_VERSION` | Kubernetes version for the PSA audit policy. Only applies when using kubernetes backend. | `latest` |
| `OBOT_SERVER_MCPPOD_SECURITY_WARN` | Pod Security Standards level to warn about for MCP namespace (privileged, baseline, or restricted). Only applies when using kubernetes backend. | `restricted` |
| `OBOT_SERVER_MCPPOD_SECURITY_WARN_VERSION` | Kubernetes version for the PSA warn policy. Only applies when using kubernetes backend. | `latest` |
| `OBOT_SERVER_DISABLE_UPDATE_CHECK` | Disable the Obot server update check. | `false` |
| `OBOT_SERVER_PRODUCT_ANALYTICS_MODE` | [Product analytics](./product-analytics.md) mode: `consent` lets admins choose, `on` forces analytics on, and `off` disables analytics. | `consent` |
| `OBOT_SERVER_HIDE_K8S_DETAILS` | Hide Kubernetes configuration details such as the Server Scheduling page from the UI. | `false` |
| `OBOT_SERVER_MCPOAUTH_CLIENT_EXPIRATION` | The expiration time for dynamically registered MCP OAuth clients. Must be a valid duration string and may include days, hours, or minutes. | `30d` |
| `OBOT_SERVER_MCPOAUTH_CLIENT_NATIVE_EXCEPTIONS` | A comma-separated list of additional Client ID Metadata Document URLs that default to the native application type when `application_type` is omitted. | - |
| `OBOT_SERVER_FORCE_DYNAMIC_CLIENT` | Force Dynamic Client Registration when Obot authenticates to remote MCP servers, even when the authorization server supports Client ID Metadata Documents. | `false` |
| `OBOT_SERVER_LLM_GATEWAY_DATA_DIR` | Where the LLM gateway keeps its generated configuration and model price data and, with SQLite, its stores and encryption key. | `$XDG_DATA_HOME/obot/llm-gateway` |
| `OBOT_SERVER_LLM_GATEWAY_POSTGRES_DSN` | Postgres database for the LLM gateway's configuration. It must be a different database from Obot's. When Obot uses Postgres and this is unset, Obot creates a `<database>_llm_gateway` database on the same server; if its database user can't create databases, create that database yourself, owned by Obot's database user, or set this, or the gateway doesn't start. | derived |
| `OBOT_SERVER_LLM_GATEWAY_ENCRYPTION_KEY` | Key that encrypts LLM gateway provider credentials at rest. When unset, a key is generated: when the gateway uses Postgres (Obot on Postgres, or `OBOT_SERVER_LLM_GATEWAY_POSTGRES_DSN` set) it is stored in Obot's database and shared by every replica; with SQLite it is stored in the data dir. Set it before the first start: changing it later makes stored provider keys unreadable, and they must be entered again. | generated |
| `OBOT_SERVER_LLM_GATEWAY_LOG_LEVEL` | Log level for the embedded LLM gateway (`debug`, `info`, `warn`, `error`). | `warn` |
| `OBOT_SERVER_LLM_GATEWAY_MODEL_DATA_URL` | Where the LLM gateway refreshes model prices and capabilities from, every 6 hours. Obot's published images and release binaries include a copy of this data; set `off` to use only that copy. | `https://upgrade-server.obot.ai/v1/model-data` |
| `OBOT_SERVER_LICENSE_KEY` | An Obot license key. It takes precedence over a key stored in the database, and the key can't be changed or removed in the UI while it's set. Obot validates it with `https://api.keygen.sh`; see [Editions and licensing](../enterprise/overview.md#how-obot-validates-a-license). | - |
| `OBOT_DEFAULT_SKILL_REPO_URL` | The default skill repository URL. Must be a full HTTPS GitHub URL (e.g. `https://github.com/org/repo`). Only used on first-time setup (before the first owner user is created). A SkillRepository resource will be created from this URL and synced automatically. | `https://github.com/obot-platform/skills` |
| `OBOT_DEFAULT_SKILL_REPO_REF` | The ref (branch, tag, or commit SHA) for the default skill repository. If empty, the repository's default branch is used. Only used on first-time setup. | - |

### Claude egress audit retention (experimental)

`OBOT_SERVER_EGRESS_AUDIT_LOG_RETENTION_DAYS` controls how many days egress audit
records are retained after their last update. The default is `90`; `0` disables
cleanup. In Helm, set this under `config.OBOT_SERVER_EGRESS_AUDIT_LOG_RETENTION_DAYS`.

The admin Audit Logs page includes a **Claude Environments** tab. Only members of the auditor
group can read request/response bodies and headers, including administrators.

See [experimental Claude Environments](../claude-environments/overview.md) for setup, access control, and audit limitations.

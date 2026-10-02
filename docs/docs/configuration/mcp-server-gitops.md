---
title: MCP Server GitOps
---

## Overview

Obot supports managing MCP servers through Git repositories, enabling GitOps workflows. Instead of manually adding MCP servers one at a time, administrators can source server configurations from Git repositories. This supports collaborative workflows with proper code review, versioning, and automated validation processes.

Catalog sources can also define [vMCPs](../functionality/virtual-mcps.md) whose components refer to entries in those sources.

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
| Bitbucket Cloud | `https://bitbucket.org/workspace/repo` or `https://bitbucket.org/workspace/repo.git` |
| Bitbucket Cloud with branch | `https://bitbucket.org/workspace/repo/my-branch` |
| Self-hosted | `https://git.example.com/org/repo.git` |

For GitHub, GitLab, and Bitbucket Cloud a `.git` suffix is optional. For self-hosted instances it is required. To specify a branch, append it after the repository name (e.g. `/my-branch` or `.git/my-branch`). GitLab subgroup repositories require the `.git` suffix to distinguish the subgroup path from a branch name. If no branch is specified, Obot uses `main`.

### Private repositories

To pull from a private repository, enter a **Personal access token** in the optional field below the URL. The token is stored securely and never returned by the API after saving.

**Required token scopes:**

- **GitHub**: `repo` (read access is sufficient)
- **GitLab**: `read_repository` (clone access) + `read_api` (pre-clone size check)
- **Bitbucket Cloud personal API token**: `read:repository:bitbucket`, belonging to a user with access to the repository. See [Atlassian's API token documentation](https://support.atlassian.com/bitbucket-cloud/docs/using-api-tokens/).
- **Bitbucket Cloud repository access token**: **Repositories → Read** on the catalog repository. Create one under the repository's **Settings → Security → Access tokens**. See [Atlassian's repository access token documentation](https://support.atlassian.com/bitbucket-cloud/docs/create-a-repository-access-token/).

For private Bitbucket Cloud repositories, enter either token type in the **Personal access token** field. Use a repository URL such as `https://bitbucket.org/workspace/repo`; no username or token is needed in the URL. Public Bitbucket Cloud repositories do not require a token.

If no per-URL token is configured, Obot falls back to the `GITHUB_AUTH_TOKEN` environment variable.

## Selecting Catalog Files

Obot recursively scans for `*.json`, `*.yaml`, and `*.yml` files. Hidden child directories such as `.git` and `.github` are always skipped, so GitHub Actions workflows can live alongside catalog entries. Keep catalog entries outside hidden directories.

To customize discovery, add these files at the catalog repository root. Both accept one pattern per line, ignoring blank lines and `#` comments.

### `.obotcatalogs`: include files

A nonempty pattern list replaces the defaults. Patterns without `/` match filenames at any depth; patterns with `/` match paths relative to the catalog root:

```text
# Matching filenames anywhere in the repository
*.mcp.yaml

# YAML files directly inside servers/
servers/*.yaml
```

### `.ignoreobotcatalogs`: exclude files or directories

Patterns match root-relative paths and override includes. Matching a directory excludes its entire subtree:

```text
scripts
renovate.json
.pre-commit-config.yaml
```

Wildcards do not cross `/`, and `**` is not recursive. To exclude a whole directory tree, list the directory itself. Include patterns must match files; a bare directory does not include its contents.

## Validating Catalog Entries

With the `obot` CLI installed, run this from your catalog repository root before pushing changes:

```sh
obot mcp validate-catalog .
```

Directory validation honors the same pattern files and hidden-directory skipping as catalog sync, then selects files for `--format`: `*.yaml` and `*.yml` for `yaml` (the default), or `*.json` for `json`. It checks the selected catalog entries and exits with an error if validation fails, making it suitable for CI.

You can also validate individual files:

```sh
obot mcp validate-catalog servers/github.yaml
```

Explicit file arguments are validated directly, without applying directory filters. Use `--format json` to require JSON input:

```sh
obot mcp validate-catalog --format json servers/github.json
obot mcp validate-catalog --format json .
```

For system MCP catalog entries, use `obot mcp validate-system-catalog` with the same `--format yaml|json` flag. Both commands default to `yaml`; other format values are rejected.

The previous names, `validate-catalog-yaml` and `validate-system-catalog-yaml`, remain available as compatibility aliases with the same flags and YAML default.

## Converting Legacy Catalog Configuration

Use `obot mcp convert-catalog` to migrate legacy `env`, header, and `serverUserType` fields to the current `config` schema in place:

```sh
obot mcp convert-catalog servers/github.yaml
obot mcp convert-catalog --format json servers/github.json
```

`--format` defaults to `yaml` and accepts `yaml` or `json`. For a directory, only files with the selected format's extensions are converted, honoring `.obotcatalogs` and `.ignoreobotcatalogs`. YAML conversion preserves comments; JSON conversion writes JSON. Unknown fields are retained, and a file that fails conversion is left unchanged. Review the changes before publishing. Composite definitions must be migrated to vMCPs separately. The previous name, `convert-catalog-yaml`, remains a compatibility alias.

## Configuration Format

MCP server configurations can use YAML or JSON files, each defining a single MCP server. The examples below use YAML. These files contain comprehensive metadata including:

- **Name and Description**: Human-readable identification
- **Tool Previews**: Documentation of available tools and their parameters
- **Metadata**: Categories, icons, repository URLs, and classification information
- **Environment Variables**: Required and optional configuration parameters
- **Resource Requirements**: Optional CPU and memory requests and limits for hosted server deployments
- **Runtime Configuration**: Deployment and connection details

For examples and reference implementations, see the official Obot MCP server repository at [github.com/obot-platform/mcp-catalog](https://github.com/obot-platform/mcp-catalog).

## YAML Configuration Structure

Each MCP server is defined in its own YAML file with the following structure:

### Basic Information

```yaml
entryKey: server-name # optional stable key within this source
name: Server Name
description: |
  Detailed description of the server's capabilities and features.
  Supports multi-line markdown formatting.
```

The optional `entryKey` field defines a stable key for this catalog entry. It must be unique within its source, DNS-friendly, and cannot contain `::`.

### vMCP definitions

Set top-level `type: vmcp` to synchronize a vMCP. An omitted or empty `type` means `entry`. A vMCP uses the [vMCP manifest fields](../functionality/virtual-mcps.md), plus an optional `entryKey` that keeps its identity stable when its display name changes:

```yaml
type: vmcp
entryKey: research
displayName: Research
components:
  - name: Search
    id: search-component
    mcpServerCatalogEntryKey: search
```

Each component's `mcpServerCatalogEntryKey` names an entry's `entryKey` in the same source. To refer to an entry in another source of the same catalog, use `sourceID::entryKey`, where `sourceID` is the source URL without `http://` or `https://` and trailing slashes. Catalog entries used as vMCP components therefore need an `entryKey`. A vMCP can be in its own file or in a YAML/JSON list alongside entries.

For example, to reference `entryKey: search` from the catalog source `https://github.com/example/shared-catalog/`:

```yaml
components:
  - name: Search
    id: search-component
    mcpServerCatalogEntryKey: github.com/example/shared-catalog::search
```

Every component must have a nonblank `id`. If a profile uses `allowedComponents`, use that ID as the map key. Keep component IDs stable across updates so saved configuration stays associated with the same component. When adopting an [orphaned vMCP](#migrating-git-synced-composites-to-vmcps), use its existing component IDs.

```yaml
type: vmcp
entryKey: research
displayName: Research
components:
  - id: search-component
    name: Search
    mcpServerCatalogEntryKey: search
profiles:
  - name: everyone
    subjects:
      - type: selector
        id: "*"
    vmcpPermissions:
      allowedComponents:
        search-component:
          allowedTools: null # Allow all enabled tools for this component.
```

A component's `configuration` sets a policy for each of its entry's configuration fields: `fixed`, `userAllowed`, or `prohibited`. A `fixed` field can use `secretBinding` instead of a clear-text `value` so Obot reads the value from a Kubernetes Secret when the component server starts:

```yaml
components:
  - id: gitlab-component
    name: GitLab
    mcpServerCatalogEntryKey: obot-gitlab
    configuration:
      - key: GITLAB_PERSONAL_ACCESS_TOKEN
        policy: fixed
        secretBinding:
          name: gitlab-secret # Kubernetes Secret name
          key: gitlab_key     # Key within that Secret
```

vMCP secret bindings follow the same rules as [catalog entry secret bindings](#kubernetes-secret-bindings) and are only available in Git catalog sources. A binding cannot be combined with `value`, and the entry's field must not define a static `value`. Catalog synced vMCPs cannot be configured through the Obot UI or API, so sync reports an error and does not create the vMCP when a required `fixed` field has neither a `value` nor a `secretBinding`.

When replacing a migrated composite with a catalog vMCP, keep the same `entryKey` and catalog source. Migration and sync use these to generate the same vMCP ID, preserving existing connections and credentials. Sync marks the vMCP as `adopted` when it takes over management. If the original entry had no `entryKey`, retain its name as the vMCP's `displayName` for matching.

For example, if the original composite had `entryKey: email` and `name: Email`, publish this replacement in the same catalog source after migration:

```yaml
type: vmcp
entryKey: email
displayName: Email
components:
  - name: Gmail
    id: gmail-component
    mcpServerCatalogEntryKey: obot-gmail
  - name: Outlook
    id: outlook-component
    mcpServerCatalogEntryKey: obot-outlook
```

The source must also contain entries with `entryKey: obot-gmail` and `entryKey: obot-outlook`. If the original composite had no `entryKey`, omit `entryKey: email` above and keep `displayName: Email` unchanged.

Rather than writing replacements by hand, use `obot mcp generate-vmcp-catalog` to generate them. See [Migrating Git-synced composites to vMCPs](#migrating-git-synced-composites-to-vmcps).

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

`unsupportedTools` is a comma-separated list of tools that are known not to work well in Obot. Obot marks these tools as unsupported and leaves them unselected by default, but this field is not an access-control restriction: a client connected directly to the MCP server can still discover them through `tools/list` and call them.

To restrict which tools clients can discover and call, create a [virtual MCP (vMCP)](../concepts/mcp-hosting.md#virtual-mcps-vmcps) and configure its exposed tools and access profiles.

### Environment Variables

```yaml
env:
  - key: ENVIRONMENT_VARIABLE
    name: Human Readable Name
    required: true
    sensitive: true
    description: Description of this variable
```

#### Static values

A field with a `value` is static configuration: users are not asked for it. Obot stores static values
in its encrypted credential store and omits them from the catalog entry returned by the API, so they
are not visible to users who can connect to the entry. The value is still clear text in the Git
repository; use a [secret binding](#kubernetes-secret-bindings) for secrets that should not be
committed. Removing a `value` from the source removes it from the entry on the next sync.

#### Selectable configuration values

Git-synced catalog entries can constrain an environment variable or remote header to a catalog-owned
set of values by adding `options`. Obot renders these fields as dropdowns and rejects configuration
values that are not listed. Option values are case-sensitive; each option requires a unique, non-empty
`value` and a non-empty display `name`. The optional `description` is shown after selection.

```yaml
env:
  - name: DigitalOcean Service
    key: DIGITALOCEAN_SERVICE
    description: Choose the DigitalOcean service to connect
    required: true
    sensitive: false
    options:
      - name: App Platform
        value: apps
        description: Deploy, manage, and monitor App Platform applications.
      - name: Kubernetes
        value: doks
        description: Manage DigitalOcean Kubernetes clusters and node pools.
```

The same `options` shape is supported under `remoteConfig.headers`. An option-backed field cannot
also define a static `value` or `secretBinding`, and options cannot be authored through the catalog
entry UI or non-Git catalog APIs. Optional fields may be left unselected; required fields must select
one of the declared values.

For `remoteConfig.urlTemplate`, declare every `${VAR}` as a required field under `env`, where it may
also define options. Template substitutions are validated selections and cannot be supplied by
overriding the derived URL directly.

```yaml
runtime: remote
remoteConfig:
  urlTemplate: https://${REGION}.api.example.com/mcp
env:
  - key: REGION
    name: Region
    required: true
    sensitive: false
    options:
      - name: United States
        value: us
      - name: Europe
        value: eu
```

### Server User Type

```yaml
serverUserType: singleUser  # Valid values: "singleUser" or "multiUser"
```

The `serverUserType` field specifies how users interact with the catalog entry:

- `singleUser`: Each user who installs this catalog entry gets their own independent MCP server instance.
- `multiUser`: The catalog entry is a template for shared multi-user deployments. An administrator or Power User+ deploys the template into a catalog or workspace, and users connect to that shared deployment through per-user MCP server instances.

Catalog entries should set this field explicitly. For compatibility with existing catalogs, some import paths normalize an omitted value to `singleUser` before validation. Any persisted value other than `singleUser` or `multiUser` is rejected at validation time.

Catalog templates support the `npx`, `uvx`, `containerized`, and `remote` runtimes.

### Composite runtime removal

Catalog entries with `runtime: composite` are no longer supported, including entries imported through GitOps. Obot converts existing composites to vMCPs when it upgrades. Replace the composite definitions in your catalog sources with vMCP definitions, as described in [Migrating Git-synced composites to vMCPs](#migrating-git-synced-composites-to-vmcps).

#### Multi-user template with shared configuration

Use `serverUserType: multiUser` when the catalog entry should create a shared deployment instead of one server per user. Deployment-level configuration, such as shared credentials, is configured on the deployed server.

```yaml
name: Shared Weather
description: Shared weather API server for the organization.
serverUserType: multiUser
runtime: uvx
uvxConfig:
  package: weather-mcp-server
env:
  - key: WEATHER_API_KEY
    name: Weather API Key
    required: true
    sensitive: true
    description: Shared API key configured on the deployed multi-user server.
```

#### Per-user headers for multi-user deployments

If each user must provide values after connecting to the shared deployment, define `multiUserConfig.userDefinedHeaders`. These values are stored on the user's MCP server instance and sent through the gateway for that user's requests.

```yaml
name: Shared Internal API
description: Shared multi-user server with per-user request headers.
serverUserType: multiUser
runtime: remote
remoteConfig:
  fixedURL: https://api.example.com/mcp
multiUserConfig:
  userDefinedHeaders:
    - key: X-User-Token
      name: User Token
      required: true
      sensitive: true
      description: Personal token used by the upstream service for this user.
```

### Resource Requirements

Catalog entries can optionally define CPU and memory requests and limits for hosted MCP server deployments. These values are useful when a specific server needs more or fewer resources than the platform defaults.

```yaml
resources:
  requests:
    cpu: 250m
    memory: 512Mi
  limits:
    cpu: "1"
    memory: 1Gi
```

Supported fields are:

- `resources.requests.cpu`
- `resources.requests.memory`
- `resources.limits.cpu`
- `resources.limits.memory`

Values use standard Kubernetes resource quantity syntax, such as `250m`, `1`, `512Mi`, or `1Gi`.

When omitted, Obot uses the configured MCP server resource defaults. When specified, catalog entry resource values override the corresponding default request or limit for servers created from that catalog entry.

If the Kubernetes runtime has MCP resource maximums configured, catalog entry resource values must be less than or equal to those maximums when the entry is created, updated, or refreshed from Git. If a previously-created MCP server already exceeds the current maximums, users can still connect to it, but configuration changes to that server must lower the resources below the maximums.

### Kubernetes Secret Bindings

Secret bindings let you wire an env var, header, or file to a key in an externally-managed Kubernetes Secret instead of asking the user to supply the value at install time.

Secret bindings are only available when Obot is using the Kubernetes MCP runtime backend.

The referenced Kubernetes Secret must be in the Obot server namespace and must have the [configured allowed secret-binding label](./server-configuration.md).

Obot checks this label when resolving bound values. If the Secret is missing the label, the binding is treated the same as a missing Secret or key. Required bound fields are reported as missing configuration.

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
- Do not set `secretBinding.adminAdded` in Git catalog YAML. Obot sets this field only on deployed servers for admin-selected bindings.

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
entryKey: github

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

## Migrating Git-synced composites to vMCPs

When Obot upgrades to a release without composite MCP servers, it converts every composite catalog entry to a [vMCP](../functionality/virtual-mcps.md) at startup. The vMCP keeps the composite's components, tool settings, access, configuration, and users' existing connections and credentials.

Composites that you created in the Obot UI need no further action. A composite that was synced from a Git source becomes an **orphaned** vMCP: Obot keeps serving it, but no catalog source defines it. To manage it through GitOps again, publish a matching vMCP definition in the same source. On its next sync, Obot adopts the vMCP instead of creating a new one, so connections and credentials carry over.

`obot mcp generate-vmcp-catalog` generates those definitions. The previous name, `generate-vmcp-catalog-yaml`, remains a compatibility alias with the same flags and YAML default.

### What the command does

```bash
obot mcp generate-vmcp-catalog <catalog-source-url> [path] [--format yaml|json] [--mode stdout|file|dir] [--overwrite]
```

The command asks Obot for every orphaned vMCP in the default catalog that was migrated from a composite synced from `<catalog-source-url>`. It then writes one `type: vmcp` catalog item for each. Each item contains:

- An explicit `entryKey`. This is the composite's original `entryKey`, or the key Obot derived from its name if it had none. Because the key is explicit, you can later rename the vMCP's `displayName` without losing its identity.
- Each component's existing `id`, name, tool prefix, allowed tools, and tool overrides.
- Configuration policies without their values. Sync keeps the fixed values Obot already stores for each component ID, so secrets never end up in your repository.
- The vMCP's profiles, including an empty list if it has none. Omitting `profiles` would give the vMCP an admin-only default profile.
- For each component, a `mcpServerCatalogEntryKey` that points to the entry the component uses. Entries in the same source are referenced by `entryKey` alone. Entries in another source of the catalog are referenced as `sourceID::entryKey`.

The command only reads from Obot. It doesn't change Obot or your repository.

### Before you begin

- **Use an administrator account.** Sign in with the CLI first (for example, `obot login --url https://obot.example.com`), or set `OBOT_BASE_URL` and `OBOT_TOKEN`.
- **Finish the upgrade and let the catalog sync.** Component references are built from the catalog entries' current sources. If a source URL changed, entries keep reporting the old URL until the catalog syncs. For example, an upgrade can move the default catalog to a new branch. Obot starts a sync automatically when an upgrade changes the default source URL. If you changed a source yourself, click **Sync** on **Admin → MCP Servers** and wait for it to finish. The command refuses to generate a definition for a component whose entry still reports a removed source.
- **Know the exact source URL the composite came from.** It must be the URL configured in Obot, including any branch path such as `/v2-schema`. The scheme (`http://` or `https://`) and trailing slashes are ignored when matching.

### Migrate a catalog source

1. **Generate the vMCP definitions** from a checkout of the catalog repository:

   ```bash
   obot mcp generate-vmcp-catalog https://github.com/example/catalog --mode dir ./vmcps
   ```

2. **Replace the composites.** In the same repository and branch, delete each `runtime: composite` definition and commit the matching generated file in its place. The vMCP must be published in the source that the composite came from: Obot derives the vMCP's identity from the catalog, source URL, and `entryKey`, so publishing it anywhere else creates a separate vMCP.
3. **Review the definitions before committing.** Don't change `entryKey` or component `id` values. Changing either breaks the match with the orphaned vMCP or its stored configuration. You can edit display names, descriptions, tool settings, and profiles.
4. **Validate the source:**

   ```bash
   obot mcp validate-catalog .
   ```

5. **Push the change** and click **Sync** on **Admin → MCP Servers**. If sync can't adopt a vMCP, the error appears next to the source on the **Git Source URLs** tab.
6. **Confirm the adoption** by running the command again. When every vMCP from the source has been adopted, the command prints:

   ```text
   No orphaned vMCPs from https://github.com/example/catalog are awaiting catalog sync.
   ```

Once a vMCP is adopted, the catalog source manages it like any other synced item. Make future changes in the source. Removing the vMCP's definition from the source deletes the vMCP.

### Output modes

Use `--format yaml|json` to choose the output format (default: `yaml`) and `--mode` to choose where it goes:

| Mode | Output |
|------|--------|
| `stdout` (default) | Every vMCP in one YAML list or JSON array, printed to standard output. |
| `file` | Every vMCP in one YAML list or JSON array, written to the file at `[path]`. |
| `dir` | Each vMCP in its own file in the `[path]` directory, named after its display name. If two vMCPs share a display name, the second file name also includes the `entryKey`. Files use `.yaml` or `.json` according to `--format`. |

```bash
obot mcp generate-vmcp-catalog https://github.com/example/catalog --mode file vmcps.yaml
obot mcp generate-vmcp-catalog https://github.com/example/catalog --format json --mode file vmcps.json
```

The command never replaces an existing file unless you pass `--overwrite`. In `stdout` mode, only the generated YAML or JSON goes to standard output and all messages go to standard error, so you can redirect the output to a file.

The command generates definitions for every vMCP it can, even when some vMCPs can't be generated. It prints a `Skipping vMCP` message to standard error for each one it skips, then exits with an error. Scripts can therefore treat a nonzero exit status as "some vMCPs still need attention".

### Troubleshooting

**`No orphaned vMCPs from <url> are awaiting catalog sync.`**
Either every orphaned vMCP from that source has already been adopted, or the URL doesn't match the source the composites were synced from. Check the URL on the **Git Source URLs** tab, including any branch path.

**`Skipping vMCP "<name>": ...`**
A catalog vMCP can only reference catalog entries that are synced from a Git source and have an `entryKey`. Fix the cause shown in the message, then run the command again:

| Message ends with | Fix |
|-------------------|-----|
| `which has no entryKey; add an entryKey to that entry so the vMCP can reference it` | Add an `entryKey` to that entry in its source, sync the catalog, and run the command again. |
| `which is no longer a source of catalog "default"; sync the catalog and generate the YAML again` | The entry still reports a source URL that the catalog no longer uses. Sync the catalog and run the command again. |
| `which is not synced from a catalog source` | The component uses an entry created in the Obot UI. Move that entry into a Git source, or keep managing this vMCP in Obot. |
| `uses multi-user MCP server ...` or `which no longer exists` | There is no catalog entry the definition can reference. Keep managing this vMCP in Obot. |

**Sync reports `component catalog entry "<sourceID>::<entryKey>" was not found`**
The reference doesn't match any entry in the catalog's current sources. This usually happens when a source URL changed after you generated the YAML, because `sourceID` includes the full source URL and branch path. Sync the catalog, then run the command again, or update the `sourceID` prefix in the published YAML.

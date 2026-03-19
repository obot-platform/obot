---
title: Workflow Sharing
---

# Workflow Sharing

Workflow sharing lets users publish a workflow from Obot Agent so it can be discovered and installed by other users as a reusable starting point.

This feature is designed for sharing complete workflow packages, not just a single markdown file. When a workflow is published, Obot stores the workflow's `SKILL.md` plus every other file in the workflow directory.

## How It Works

Each published workflow has:

- A stable artifact ID
- A workflow name taken from the `SKILL.md` frontmatter
- A generated display name based on that workflow name
- A latest version number
- A visibility setting: `private` or `public`

Publishing the same workflow name again as the same user creates a new version of the existing published workflow instead of creating a separate entry.

Two different users can publish workflows with the same name. They will be stored as separate published workflows with different IDs.

## Publishing a Workflow

Workflow sharing is exposed through the workflow tools available to Obot Agent's Nanobot integration.
To publish a workflow, simply ask the agent to publish it.

When publishing succeeds:

- The first publish creates version `1`
- Republishing the same workflow creates version `2`, `3`, and so on
- The published package includes the entire workflow directory
- The shared workflow starts with `private` visibility

## Discovering Shared Workflows

Agents also have a tool that they can use to search for published workflows.

Search matches against:

- The workflow name
- The generated display name
- The workflow description

Search results include the workflow ID, name, display name, description, latest version, visibility, and author email when available.

## Installing a Shared Workflow

Agents have a tool to install workflows that they found using the search tool.

Installation behavior:

- Installing without a version downloads the latest version
- Installing with a version downloads that specific version
- The workflow is extracted into `workflows/<name>/`
- If a local workflow with the same name already exists, the user is asked to confirm the overwrite
- After installation, the workflow is immediately available for use

:::note
The current install flow relies on the runtime having `unzip` available and does not support Windows-based runtimes.
:::

## Visibility and Access

Published workflows support two visibility levels:

- `private`: Only the workflow owner and admins can view or download it
- `public`: Other users can discover it in search results and install it

:::important
Published workflows are always `private` by default. In order to become `public`, the user needs to go to the Obot UI and explicitly make it public. The Obot Agent is unable to change the visibility of published workflows.
:::

Access rules are enforced on the Obot side:

- Search results include public workflows plus your own private workflows
- Admins can see all workflows
- Private workflows are hidden from other users
- Only the owner or an admin can change metadata or delete a published workflow

## Versioning

Versioning is per workflow name and per publisher.

That means:

- Your first publish of `code-review` is version `1`
- Republishing your `code-review` workflow creates version `2`
- Another user publishing their own `code-review` workflow creates a separate published workflow with its own version history

Older versions remain downloadable by version number as long as the published workflow still exists.

## Operational Requirements

Workflow sharing depends on two pieces of platform configuration:

- Nanobot integration must be enabled so the workflow tools are available in Obot Agent
- Obot must have storage available for published workflow ZIP files

### Nanobot Integration

Workflow sharing relies on Obot's Nanobot-backed workflow tools.

| Environment Variable | Description | Default |
|---------------------|-------------|---------|
| `OBOT_SERVER_NANOBOT_INTEGRATION` | Enables the Nanobot integration used by workflow publishing, search, and install flows. | `true` |

### Published Workflow Storage

Obot stores published workflows separately from workspace files.

If you do not configure a cloud storage provider, Obot falls back to local disk storage under its data directory. That is acceptable for local development and single-node testing, but it is not recommended for highly available or ephemeral deployments.

For production, configure one of the supported storage providers for published workflows:

| Provider | Required Settings |
|----------|-------------------|
| `s3` | `OBOT_ARTIFACT_STORAGE_PROVIDER=s3`, `OBOT_ARTIFACT_STORAGE_BUCKET`, `OBOT_ARTIFACT_S3_REGION` |
| `custom` | `OBOT_ARTIFACT_STORAGE_PROVIDER=custom`, `OBOT_ARTIFACT_STORAGE_BUCKET`, `OBOT_ARTIFACT_S3_ENDPOINT`, `OBOT_ARTIFACT_S3_REGION`, `OBOT_ARTIFACT_S3_ACCESS_KEY_ID`, `OBOT_ARTIFACT_S3_SECRET_ACCESS_KEY` |
| `gcs` | `OBOT_ARTIFACT_STORAGE_PROVIDER=gcs`, `OBOT_ARTIFACT_STORAGE_BUCKET`, optionally `OBOT_ARTIFACT_GCS_SERVICE_ACCOUNT_JSON` |
| `azure` | `OBOT_ARTIFACT_STORAGE_PROVIDER=azure`, `OBOT_ARTIFACT_STORAGE_BUCKET`, `OBOT_ARTIFACT_AZURE_STORAGE_ACCOUNT`, and Azure identity settings if not using default credentials |

Provider-specific behavior:

- `s3` can use ambient AWS credentials or explicit access keys
- `custom` is for S3-compatible systems such as MinIO or Cloudflare R2
- `gcs` can use Application Default Credentials or an inline service account JSON document
- `azure` can use default Azure credentials or explicit client credentials

## Deployment Guidance

For Docker or single-node development:

- The default local artifact store is usually sufficient
- Keep Obot's data volume persistent if you want shared workflows to survive container replacement

For Kubernetes or multi-replica production:

- External object storage is the recommended production option
- If you do not want object storage, keep the chart's `persistence` PVC enabled because it mounts `/data`, which includes `/data/.local/share/obot/published-artifacts`
- Use `ReadWriteOnce` only for a single Obot replica
- Use `ReadWriteMany` for multi-replica Obot deployments so every replica can access the same artifact files
- Treat published workflow storage the same way you treat other persistent user-generated platform data

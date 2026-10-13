---
title: Install Claude Environments
description: Enable experimental Claude Environments with Helm and configure snapshot storage and PostgreSQL.
---

:::warning Experimental
Use an evaluation cluster first. Complete the [cluster prerequisites](./cluster-requirements.md) before enabling the chart. The feature is disabled by default.
:::

## Enable the feature

Use an Obot build and chart that contain this feature. Preserve your normal database, authentication, encryption, ingress, and other Obot deployment settings. The examples below are Nushell commands from a checkout containing `./chart`; they show the **additional** configuration and are not a replacement for your normal Obot installation command.

For an existing S3 bucket and an existing credentials Secret:

```nu
let substrate_args = [
  '--set' 'substrate.enabled=true'
  '--set-string' 'config.OBOT_SERVER_SUBSTRATE_SNAPSHOT_LOCATION=s3://YOUR_BUCKET/claude/'
  '--set-string' 'substrate.storage.env.AWS_REGION=us-east-1'
  '--set-string' 'substrate.storage.existingSecret=substrate-snapshot-credentials'
]
# Append ...$substrate_args to your normal helm upgrade command.
helm upgrade --install obot ./chart --namespace obot --create-namespace ...$substrate_args --wait --wait-for-jobs --timeout 10m
```

The Secret must exist in `ate-system` and contain `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY`. For a fresh namespace owned by this release, prepare it before creating the Secret:

```nu
kubectl create namespace ate-system
kubectl label namespace ate-system app.kubernetes.io/managed-by=Helm
kubectl annotate namespace ate-system meta.helm.sh/release-name=obot meta.helm.sh/release-namespace=obot
```

Replace release name/namespace for your installation. If the namespace already exists, verify who owns it before adding ownership metadata; do not adopt another installation's resources. Alternatively, supply the two keys through `substrate.storage.credentials` and Helm creates the namespace and Secret in the same installation. Never commit actual credentials to documentation or source control; command-line values can also be retained in shell history and Helm release data.

With workload identity, omit both `existingSecret` and `credentials`, configure the cloud IAM bindings independently, and confirm that the Substrate pods can access the bucket. Do not specify both static-secret options.

### R2 and other S3-compatible services

Keep `substrate.storage.backend=s3`, the `s3://` snapshot URI, and add endpoint settings appropriate to the service. For R2:

```nu
let r2_args = [
  '--set-string' 'substrate.storage.env.AWS_ENDPOINT_URL=https://ACCOUNT_ID.r2.cloudflarestorage.com'
  '--set-string' 'substrate.storage.env.AWS_REGION=auto'
  '--set-string' 'substrate.storage.env.AWS_S3_USE_PATH_STYLE=true'
]
```

Append `...$r2_args` as well as the feature arguments and your credentials configuration to the Helm command. The bucket must already exist.

### GCS

Set `substrate.storage.backend=gcs` and a `gs://YOUR_BUCKET/claude/` snapshot URI. Use application default credentials, such as GKE Workload Identity, for the `ate-system/atelet` and `ate-system/ate-api-server` service accounts. Configure snapshot object read/write/delete and bucket metadata access. Do not assume that IAM permissions on the Obot pod grant access to these other pods.

### PostgreSQL

Without an external DSN, Helm deploys PostgreSQL with an 8 GiB persistent volume. Use `substrate.postgres.storageClass` and `substrate.postgres.storageSize` to customize it.

For an external database, add:

```nu
let database_args = [
  '--set-literal' 'substrate.postgres.dsn=postgres://USER:PASSWORD@HOST:5432/substrate?sslmode=require'
  '--set-string' 'substrate.postgres.role=DATABASE_ROLE'
]
```

The role is the database role used for schema management and runtime access. It must be able to apply Substrate migrations. The DSN is stored in a Kubernetes Secret; keep it out of `config`. Changing between bundled and external PostgreSQL does not migrate data. Use a dedicated Substrate database, not Obot's existing database.

## Images and internal connections

The chart selects the published `v0.1.0` Substrate component images under `ghcr.io/obot-platform/substrate`. The default runner is the published `ghcr.io/obot-platform/obot-claude-runner:v0.1.0` image, pinned by its multi-platform digest in Obot. You do not need to build these images for an ordinary evaluation.

For custom images, use the [substrate-images repository](https://github.com/obot-platform/substrate-images). Keep the worker, control plane, generated API clients, and chart compatible. A worker image accepts a tag; a runner image must use `repository@sha256:FULL_DIGEST`. Existing environments keep their saved images when deployment defaults change.

Helm supplies the Substrate API address and internal Obot Service URL. Certificate paths, the API token path, and credential-provider port are fixed internally. No public Substrate ingress or separate Claude orchestrator Deployment is needed.

| Helm value | Purpose / default |
|---|---|
| `substrate.enabled` | Enable the integration and bundled runtime; `false` by default. |
| `config.OBOT_SERVER_SUBSTRATE_SNAPSHOT_LOCATION` | Required cluster-wide `s3://` or `gs://` snapshot URI. |
| `config.OBOT_SERVER_SUBSTRATE_NAMESPACE` | Environment worker namespace; `obot-agents`. |
| `config.OBOT_SERVER_SUBSTRATE_WORKER_IMAGE` | Optional new-environment default; empty derives from the chart's repository and version. |
| `config.OBOT_SERVER_SUBSTRATE_RUNNER_IMAGE` | Optional digest-pinned new-environment default; empty uses Obot's built-in runner. |
| `substrate.image.repository` | Component repository prefix; `ghcr.io/obot-platform/substrate`. |
| `substrate.imagePullSecrets` | Registry Secrets already present in both Substrate namespaces. |
| `substrate.apiServer.replicas` | API replicas; `2`. Set `1` for a smaller evaluation deployment. |
| `substrate.atelet.nodeSelector` | Nodes running atelet; defaults to Linux nodes. |
| `substrate.storage.backend` | `s3` by default; `gcs` is also supported. |
| `substrate.storage.existingSecret` / `credentials` / `env` | Snapshot authentication and provider environment settings. |
| `substrate.postgres.dsn` / `role` | External database configuration; empty DSN selects bundled PostgreSQL. |
| `substrate.otel.endpoint` | Optional OTLP collector; upstream otherwise tries localhost and can log exporter warnings. |
| `config.OBOT_SERVER_EGRESS_AUDIT_LOG_RETENTION_DAYS` | Audit retention; `90`, or `0` to disable audit cleanup. |

Worker replicas, actor limits, idle timeout, session retention, allowed hosts, and secrets are configured per environment in the UI, not through the retired `substrate.poc.*` or `substrate.claude.*` values. Do not carry those values into a new installation.

## Verify and continue

```nu
kubectl get pods -n ate-system
kubectl get pods -n podcertificate-controller-system
kubectl get pods -n obot
kubectl get workerpools -n obot-agents
```

No environment worker pool is expected until an administrator creates an environment. Bootstrap Jobs create signing material and install custom resources. A Helm timeout is not proof of successful installation: inspect events and certificate projection failures before continuing.

Next, [create a Claude environment and map users](./administration.md), then run the [acceptance checks](./operations.md#acceptance-checks).

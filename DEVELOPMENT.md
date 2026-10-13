# Developing Obot

What follows is a rundown on different ways to run and develop Obot, its UI and its tools locally.

## Running Obot

The easiest way to run Obot locally is to run `make dev`. This will launch three processes: the API server, admin UI, and user UI. Opening `http://localhost:8080/admin/` will launch the admin UI. Changing the UI code will update the UI automatically. Changing any of the Go code requires restarting the server.

## Building and Running the Obot Docker Image

Obot is ultimately packaged into an image for distribution. You can build said image with `docker build -t my-obot .`, and then run the image via `docker run -p 8080:8080 my-obot`.

## Debugging Obot

It is possible to run the server and/or UIs in and IDE for debugging purposes. These steps layout what is necessary for JetBrains IDEs, but an equivalent process can be used with VSCode-based editors.

### Server

To run the server in GoLand:
1. Create a new "Go Build" configuration.
2. In the "Program Arguments" section, enter `server --dev-mode`.

Then you're ready to run or debug this target.

### User UI

To run the User UI in GoLand or WebStorm:
1. Create a new "npm" build.
2. In the "package.json" dropdown, select the `package.json` file in the `ui/user` directory.
3. In the "Command" dropdown, select `run`.
4. In the "Scripts" dropdown, select `dev`.
5. In the "Environment" section, enter `VITE_API_IN_BROWSER=true`.

Then you're ready to run or debug this target.

## Developing Obot Providers

Obot has a set of providers. These are in the repo `github.com/obot-platform/providers`. By default, Obot will pull the providers' configuration from this repo. However, when developing tools in this repo, you can follow these steps to use a local copy.

1. Clone `github.com/obot-platform/providers` to your local machine.
2. In the root directory of the tools repo on your local machine, run `make build-images`.
3. Run the Obot server, either with `make dev` or in your IDE, with the `OBOT_SERVER_PROVIDER_REGISTRIES` environment variable set to `<local-tools-fork-root-directory>`; e.g. If you cloned the tools repo to the directory "above" the Obot repo, you'd use `OBOT_SERVER_PROVIDER_REGISTRIES='../providers' make dev`.

Now, any time one of these tools is run, your local copy will be used.

> [!IMPORTANT]
> Any time you change a Go based tool in your local repo, you must run `make build` in the tools repo for the changes to take effect with Obot.

> [!NOTE]
> Provider definitions and metadata are only synced to Obot every hour. Therefore, if you make a change to the provider on your local machine, it may not reflect immediately in Obot.

## Obot Server Dev Mode

In the description above for running the server in an IDE, the `--dev-mode` flag is used. This flag is also used when running the server with `make dev`. This does a few things, the most helpful of which is to give you access to the database via `kubectl`. The kubeconfig is located at `tools/devmode-kubeconfig`.

For example, from the root directory of the obot repo, you can list all agents in your setup with `kubectl --kubeconfig tools/devmode-kubeconfig get agents`.

## Local Jaeger

Obot already supports standard OpenTelemetry exporters. For local tracing with Jaeger:

1. Start Jaeger:
```bash
make otel-jaeger-up
```
2. Point Obot at Jaeger before running `make dev` or starting the server in your IDE:
```bash
export OTEL_TRACES_EXPORTER=otlp
export OTEL_METRICS_EXPORTER=none
export OTEL_LOGS_EXPORTER=none
export OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
export OTEL_SERVICE_NAME=obot
export OTEL_TRACES_SAMPLER=always_on
```
3. Open Jaeger at `http://localhost:16686`.

Jaeger also exposes OTLP gRPC on `localhost:4317` and OTLP HTTP on `localhost:4318`, so Nanobot can be pointed at the same local instance.

Useful commands:

```bash
make otel-jaeger-up
make otel-jaeger-logs
make otel-jaeger-down
```

## Resetting

There may be times when you want to completely wipe your setup and start fresh. The location of data and caches is dependent on your system. For Mac or Linux, you can run the respective command in the root of the obot repo on your local machine.

On Mac:
```bash
rm -rf ~/Library/Application\ Support/obot &&
rm -rf ~/Library/Caches/obot &&
rm obot.db
```

On Linux:
```bash
rm -rf ~/.local/share/obot &&
rm -rf ~/.cache/obot &&
rm obot.db
```

## Serving the Documentation

The documentation for Obot is in the main repo. You can serve the documentation from your local machine by running `make serve-docs` in the root of the obot repo.

## Other Configuration

Obot is configured via environment variables. You can see the relevant environment variables by building the binary (as above) and running `./bin/obot server --help`. There is also documentation available. You can serve the documentation locally as above.

## Claude Environments on k3d (experimental)

Claude Environments requires Kubernetes, including certificate APIs and kubelet projections that are not enabled in a default K3s 1.36 cluster. This setup runs Obot and Substrate inside a dedicated local k3d cluster. It uses the K3s version and feature gates exercised during development; it is not a production deployment guide. All commands in this section use **Bash**, from the repository root.

### Prerequisites

- Docker with Linux containers, k3d, kubectl, and Helm (with `--set-literal` support).
- Enough Docker CPU, memory, and disk for Kubernetes, Obot, Substrate, and workers. Each environment defaults to a worker with 2 CPUs and 4 GiB of memory, in addition to the other services.
- An existing S3-compatible snapshot bucket and credentials with object read/write/delete access. The example supports AWS S3 or Cloudflare R2; the chart does not create the bucket.
- A Claude Team or Enterprise organization with self-hosted environments enabled and permission to create an environment key.

Use a cluster without another Substrate installation: its namespaces, certificate signers, and CRDs are shared cluster resources.

### 1. Create the cluster

```bash
export CLUSTER_NAME=obot-substrate

k3d cluster create "$CLUSTER_NAME" \
  --image rancher/k3s:v1.36.4-k3s1 \
  --servers 1 \
  --agents 0 \
  --k3s-arg '--kube-apiserver-arg=runtime-config=certificates.k8s.io/v1beta1=true@server:*' \
  --k3s-arg '--kube-apiserver-arg=feature-gates=ClusterTrustBundle=true,PodCertificateRequest=true@server:*' \
  --k3s-arg '--kubelet-arg=feature-gates=ClusterTrustBundle=true,ClusterTrustBundleProjection=true,PodCertificateRequest=true@server:*' \
  --wait

kubectl config use-context "k3d-$CLUSTER_NAME"
kubectl get nodes -o wide
kubectl api-resources --api-group=certificates.k8s.io
kubectl get storageclass
```

API discovery should list `clustertrustbundles` and `podcertificaterequests`. K3s supplies the default `local-path` StorageClass for this development setup. If you add separate k3d agent nodes, pass the same kubelet feature gates to them using an additional `--k3s-arg '...@agent:*'` argument at cluster creation. Enabling the API server gates alone is insufficient.

### 2. Build and import Obot

Use a new image tag on each rebuild so Kubernetes starts the new image when you upgrade the release.

```bash
export OBOT_IMAGE_TAG="substrate-dev-$(date +%Y%m%d%H%M%S)"
docker build -t "obot:$OBOT_IMAGE_TAG" .
k3d image import "obot:$OBOT_IMAGE_TAG" --cluster "$CLUSTER_NAME"
```

The chart uses published Substrate and runner images by default. You only need to build those separately when changing their implementation; see [substrate-images](https://github.com/obot-platform/substrate-images).

### 3. Configure snapshot storage and install

Replace the bucket and region below. Enter credentials at the prompts instead of putting literal credentials in shell history. Helm still stores supplied values in its release Secrets.

```bash
export SNAPSHOT_BUCKET=your-existing-bucket
export SNAPSHOT_REGION=us-east-1
read -r -p 'Snapshot access key ID: ' SNAPSHOT_ACCESS_KEY_ID
read -r -s -p 'Snapshot secret access key: ' SNAPSHOT_SECRET_ACCESS_KEY
printf '\n'

# AWS S3 needs no custom endpoint.
snapshot_endpoint_args=()

# For R2, uncomment these lines and replace YOUR_ACCOUNT_ID:
# SNAPSHOT_REGION=auto
# snapshot_endpoint_args=(
#   --set-string 'substrate.storage.env.AWS_ENDPOINT_URL=https://YOUR_ACCOUNT_ID.r2.cloudflarestorage.com'
#   --set-string 'substrate.storage.env.AWS_S3_USE_PATH_STYLE=true'
# )

helm upgrade --install obot ./chart \
  --kube-context "k3d-$CLUSTER_NAME" \
  --create-namespace --namespace obot \
  --set-string image.repository=obot \
  --set-string image.tag="$OBOT_IMAGE_TAG" \
  --set image.pullPolicy=IfNotPresent \
  --set replicaCount=1 \
  --set dev.useEmbeddedDb=true \
  --set persistence.enabled=true \
  --set ingress.enabled=false \
  --set service.type=ClusterIP \
  --set-string config.OBOT_SERVER_HOSTNAME=http://localhost:8080 \
  --set config.OBOT_SERVER_ENABLE_AUTHENTICATION=true \
  --set substrate.enabled=true \
  --set substrate.apiServer.replicas=1 \
  --set substrate.storage.backend=s3 \
  --set-string "config.OBOT_SERVER_SUBSTRATE_SNAPSHOT_LOCATION=s3://$SNAPSHOT_BUCKET/claude/" \
  --set-string "substrate.storage.env.AWS_REGION=$SNAPSHOT_REGION" \
  --set-literal "substrate.storage.credentials.AWS_ACCESS_KEY_ID=$SNAPSHOT_ACCESS_KEY_ID" \
  --set-literal "substrate.storage.credentials.AWS_SECRET_ACCESS_KEY=$SNAPSHOT_SECRET_ACCESS_KEY" \
  "${snapshot_endpoint_args[@]}" \
  --wait --wait-for-jobs --timeout 10m
```

This uses Obot's development-only embedded PostgreSQL and a separate bundled PostgreSQL for Substrate, with local persistent volumes. To use an external Obot database instead, set `dev.useEmbeddedDb=false` and supply `secret.OBOT_SERVER_DSN`. Do not point both applications at the same database. Helm automatically configures the internal Substrate address and Obot URL.

### 4. Open Obot and create an environment

```bash
kubectl get pods -n obot
kubectl get pods -n ate-system
kubectl get pods -n podcertificate-controller-system

# Retrieve the generated bootstrap login token.
kubectl get secret obot-config -n obot \
  -o jsonpath='{.data.OBOT_BOOTSTRAP_TOKEN}' | base64 --decode
printf '\n'

# Keep this running while using the browser.
kubectl port-forward -n obot service/obot-obot 8080:80
```

Open <http://localhost:8080> and use the bootstrap token to sign in. Configure authentication and Obot users, then follow [environment administration](docs/docs/claude-environments/administration.md) to create an environment from its Claude environment key, assign profiles and vMCPs, and map Claude accounts to Obot users. Workers are created when an environment is configured; an empty worker namespace before that is expected. Anthropic provisioning uses an outbound connection from Obot, so it does not require a publicly reachable localhost ingress.

### Troubleshooting and rebuilding

- **Certificate volumes fail with `unimplemented`:** verify the kubelet gates as well as the API server gates. A normal k3d cluster without these settings is insufficient.
- **Pods remain Pending:** inspect `kubectl get events -A --sort-by=.lastTimestamp` for insufficient resources or unbound volumes. Check Docker's resource allocation and the default StorageClass.
- **Obot has `ImagePullBackOff`:** confirm the exact image tag was imported into this cluster and that the Helm command uses the same tag.
- **Snapshot or resume errors:** verify the bucket, endpoint, credentials, and outbound access from the cluster. A successful Helm install alone does not verify snapshot restoration.
- **After changing Obot:** repeat the build/import step with a fresh tag, then rerun the Helm command with your storage configuration. Restart the port-forward if it disconnects during rollout.

Run the [acceptance checks](docs/docs/claude-environments/operations.md#acceptance-checks), including suspend/resume, before relying on the setup. See [cluster requirements](docs/docs/claude-environments/cluster-requirements.md) for node access and networking details.

## Running Obot Locally with Kubernetes

MCP server containers run in Kubernetes and need to reach your local Obot process. This requires [Telepresence](https://www.telepresence.io/) to bridge the network between your Mac and the cluster.

### Prerequisites

- [Rancher Desktop](https://rancherdesktop.io/) (or another local Kubernetes setup)
- [Telepresence](https://www.telepresence.io/docs/install/) v2.x
- Local images loaded into containerd (see below)

### 1. Load local images into containerd

Rancher Desktop uses containerd, not Docker's image store. Load any locally built images with:

```bash
docker save <image>:local | nerdctl --address /var/run/docker/containerd/containerd.sock load
```

### 2. Configure the cluster namespaces

The `obot-mcp` namespace is where MCP server pods run. It must exist with PSA set to `privileged` (required for Telepresence's network init container):

```bash
kubectl create namespace obot-mcp --dry-run=client -o yaml | kubectl apply -f -
kubectl label namespace obot-mcp \
  pod-security.kubernetes.io/enforce=privileged \
  pod-security.kubernetes.io/audit=restricted \
  pod-security.kubernetes.io/warn=restricted \
  --overwrite
```

The `default` namespace also needs PSA set to `privileged` for Telepresence:

```bash
kubectl label namespace default \
  pod-security.kubernetes.io/enforce=privileged \
  pod-security.kubernetes.io/audit=restricted \
  pod-security.kubernetes.io/warn=restricted \
  --overwrite
```

### 3. Set up Telepresence and intercept

Use the Makefile target to create/update the intercept target, reconnect Telepresence, restart the target deployment, and create the intercept in one step:

```bash
make telepresence-setup
```

This target runs:

```bash
kubectl create deployment obot-upstream --image=alpine --dry-run=client -o yaml -- sleep infinity | kubectl apply -f -
kubectl create service clusterip obot-upstream --tcp=8080:8080 --dry-run=client -o yaml | kubectl apply -f -
kubectl patch svc obot-upstream --type='json' -p='[{"op":"replace","path":"/spec/ports/0/name","value":"http"}]'
kubectl apply -f tools/obot-proxy.yaml
telepresence quit -s
telepresence connect
kubectl rollout restart deployment/obot-upstream
telepresence intercept obot-upstream -p 8080:8080
```

`obot-upstream` is the Telepresence intercept target — traffic to it routes to your local port 8080. `tools/obot-proxy.yaml` deploys an nginx pod as the `obot` Service (port 80). Pods reach Obot at `http://obot.default.svc.cluster.local` → nginx → `obot-upstream` (Telepresence) → your local process. nginx rewrites `http://localhost:8080` → `http://obot.default.svc.cluster.local` in response bodies so that OAuth metadata URLs are correct for pods, while your browser continues to use `http://localhost:8080` directly.

Verify the intercept is `ACTIVE` with `telepresence list`.

### 4. Configure Obot environment variables

```bash
export OBOT_SERVER_MCPRUNTIME_BACKEND='k8s'
export OBOT_SERVER_SERVICE_NAME=obot
export OBOT_SERVER_SERVICE_NAMESPACE=default
```

### Troubleshooting

- **`ImagePullBackOff`**: Image isn't in containerd — re-run `nerdctl load`.
- **`timed out waiting for MCP server to be ready: <url>`**: The URL in the error shows what Obot is trying to reach. If it's `*.svc.kubernetes` instead of `*.svc.cluster.local`, check `OBOT_SERVER_MCPCLUSTER_DOMAIN`.
- **Telepresence `NO_AGENT`**: Pod was created before intercept — run `kubectl rollout restart deployment/obot-upstream`.
- **PSA violations on `tel-agent-init`**: Namespace enforce level must be `privileged` (step 2 above).
- **Stale intercept conflict** (`conflict with intercept ... on port 8080`): A previous intercept is stuck in the Traffic Manager. Reset it with:
  ```bash
  kubectl delete pod -n ambassador -l app=traffic-manager
  kubectl rollout restart deployment/obot-upstream
  telepresence connect --namespace default
  telepresence intercept obot-upstream -p 8080:8080
  ```

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

## Developing Obot Tools

Obot has a set of packaged tools. These tools are in the repo `github.com/obot-platform/tools`. By default, Obot will pull the tools from this repo. However, when developing tools in this repo, you can follow these steps to use a local copy.

1. Clone `github.com/obot-platform/tools` to your local machine.
2. In the root directory of the tools repo on your local machine, run `make build`.
3. Run the Obot server, either with `make dev` or in your IDE, with the `GPTSCRIPT_TOOL_REMAP` environment variable set to `github.com/obot-platform/tools=<local-tools-fork-root-directory>`; e.g. If you cloned the tools repo to the directory "above" the Obot repo, you'd use `GPTSCRIPT_TOOL_REMAP='github.com/obot-platform/tools=../tools' make dev`.

Now, any time one of these tools is run, your local copy will be used.

> [!IMPORTANT]
> Any time you change a Go based tool in your local repo, you must run `make build` in the tools repo for the changes to take effect with Obot.

> [!NOTE]
> Tool definitions and metadata are only synced to Obot every hour. Therefore, if you make a change to the tool in your local machine, it may not reflect immediately in Obot. Rest assured that the latest version is used when running the tool.

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

## Obot Credentials

The GPTScript credentials for Obot are, by default, stored in a SQLite database called `obot-credentials.db` in the root of the obot repo. You can use the `sqlite3` CLI to inspect the database directly: `sqlite3 obot-credentials.db`.

## Resetting

There may be times when you want to completely wipe your setup and start fresh. The location of data and caches is dependent on your system. For Mac or Linux, you can run the respective command in the root of the obot repo on your local machine.

On Mac:
```bash
rm -rf ~/Library/Application\ Support/obot &&
rm -rf ~/Library/Application\ Support/gptscript &&
rm -rf ~/Library/Caches/obot &&
rm -rf ~/Library/Caches/gptscript &&
rm obot.db obot-credentials.db
```

On Linux:
```bash
rm -rf ~/.local/share/obot &&
rm -rf ~/.local/share/gptscript &&
rm -rf ~/.cache/obot &&
rm -rf ~/.cache/gptscript &&
rm obot.db obot-credentials.db
```

## Serving the Documentation

The documentation for Obot is in the main repo. You can serve the documentation from your local machine by running `make serve-docs` in the root of the obot repo.

## Other Configuration

Obot is configured via environment variables. You can see the relevant environment variables by building the binary (as above) and running `./bin/obot server --help`. There is also documentation available. You can serve the documentation locally as above.

## Running Obot Locally with Kubernetes (Nanobot Agents)

Nanobot agent containers run in Kubernetes and need to reach your local Obot process. This requires [Telepresence](https://www.telepresence.io/) to bridge the network between your Mac and the cluster.

### Prerequisites

- [Rancher Desktop](https://rancherdesktop.io/) (or another local Kubernetes setup)
- [Telepresence](https://www.telepresence.io/docs/install/) v2.x
- Local images loaded into containerd (see below)

### 1. Load local images into containerd

Rancher Desktop uses containerd, not Docker's image store. Load any locally built images with:

```bash
docker save nanobot:local | nerdctl --address /var/run/docker/containerd/containerd.sock load
docker save nanobot-agent:local | nerdctl --address /var/run/docker/containerd/containerd.sock load
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
kubectl create deployment obot --image=alpine --dry-run=client -o yaml -- sleep infinity | kubectl apply -f -
kubectl create service clusterip obot --tcp=80:8080 --dry-run=client -o yaml | kubectl apply -f -
kubectl patch svc obot --type='json' -p='[{"op":"replace","path":"/spec/ports/0/name","value":"http"}]'
telepresence quit -s
telepresence connect
kubectl rollout restart deployment/obot
telepresence intercept obot -p 8080:80
```

The service exposes port 80 so pods can reach Obot at `http://obot.default.svc.cluster.local` with no explicit port in URLs. The intercept maps service port 80 to your local port 8080.

The user UI dev server allows `*.svc.cluster.local` in development, so namespace/service changes for local k8s + Telepresence flows do not require manual `vite.config.ts` edits.

Verify the intercept is `ACTIVE` with `telepresence list`.

### 4. Configure Obot environment variables

```bash
export OBOT_SERVER_MCPRUNTIME_BACKEND='k8s-local'
export OBOT_SERVER_SERVICE_NAME=obot
export OBOT_SERVER_SERVICE_NAMESPACE=default

# optional if using locally-built Nanobot images
export OBOT_SERVER_NANOBOT_AGENT_IMAGE='nanobot-agent:local'
export OBOT_SERVER_MCPREMOTE_SHIM_BASE_IMAGE='nanobot:local'
```

With this setup, `TransformObotHostname` rewrites all URLs injected into pod secrets from `http://localhost:8080` to `http://obot.default.svc.cluster.local` (derived automatically from `SERVICE_NAME` and `SERVICE_NAMESPACE`), which Telepresence routes back to your local process.

### Troubleshooting

- **`ImagePullBackOff`**: Image isn't in containerd — re-run `nerdctl load`.
- **`timed out waiting for MCP server to be ready: <url>`**: The URL in the error shows what Obot is trying to reach. If it's `*.svc.kubernetes` instead of `*.svc.cluster.local`, check `OBOT_SERVER_MCPCLUSTER_DOMAIN`.
- **Telepresence `NO_AGENT`**: Pod was created before intercept — run `kubectl rollout restart deployment/obot`.
- **PSA violations on `tel-agent-init`**: Namespace enforce level must be `privileged` (step 2 above).
- **Stale intercept conflict** (`conflict with intercept ... on port 8080`): A previous intercept is stuck in the Traffic Manager. Reset it with:
  ```bash
  kubectl delete pod -n ambassador -l app=traffic-manager
  kubectl rollout restart deployment/obot
  telepresence connect --namespace default
  telepresence intercept obot -p 8080:80
  ```

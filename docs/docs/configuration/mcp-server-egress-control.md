---
title: MCP Server Egress Control
---

# MCP Server Egress Control

MCP server egress control restricts which external domains Kubernetes-hosted MCP servers can reach. It is intended for production deployments where MCP servers may run third-party code and should only be allowed to call known external services.

When this feature is enabled, individual MCP servers can be configured with a whitelist of allowed domains. Enforcement is handled by an external controller that Obot deploys using Helm. Currently, the only supported provider for this feature is Aviatrix.
The Aviatrix provider translates the whitelist of domains for an MCP server into an Aviatrix `FirewallPolicy` in the MCP runtime namespace, that targets the pod for that MCP server. Aviatrix Distributed Cloud Firewall (DCF) then enforces the generated policy.

There will be other provider options besides Aviatrix in the future.

## Prerequisites

Before enabling MCP server egress control, make sure:

- Obot is using the Kubernetes MCP runtime backend.
- Aviatrix [Distributed Cloud Firewall for Kubernetes](https://docs.aviatrix.com/documentation/latest/security/dcf-kubernetes.html?expand=true) is already configured for the Kubernetes cluster that runs Obot MCP servers.
- The Aviatrix `FirewallPolicy` CRD is installed in the cluster. The required CRD group is `networking.aviatrix.com`, kind `FirewallPolicy`.
- Aviatrix DCF can discover the cluster and apply Kubernetes firewall policies. See the Aviatrix [DCF overview](https://docs.aviatrix.com/documentation/latest/security/dcf-overview.html) for the broader enforcement model.

:::info
Obot's built-in Kubernetes `NetworkPolicy` is separate from this feature. That policy restricts private and reserved IP ranges, but it is not domain-aware. MCP server egress control adds per-server domain allowlists through Aviatrix.
:::

## Enable the Aviatrix provider

Add the Aviatrix provider chart values to your Obot Helm values:

```yaml
config:
  OBOT_SERVER_MCPNETWORK_POLICY_PROVIDER_CHART_REPO: "https://charts.obot.ai"
  OBOT_SERVER_MCPNETWORK_POLICY_PROVIDER_CHART_NAME: "aviatrix-network-policy-controller"
  OBOT_SERVER_MCPNETWORK_POLICY_PROVIDER_CHART_VERSION: "v0.0.1"
```

Then install or upgrade Obot:

```bash
helm upgrade --install obot obot/obot \
  --namespace obot \
  --create-namespace \
  -f values.yaml
```

When these values are set, Obot installs the Aviatrix provider chart as the Helm release `obot-network-policy-provider` in the namespace where Obot is installed. The chart is configured to manage egress-control resources for the MCP runtime namespace.

## Configure default egress behavior

By default, an MCP server with no egress domains configured is treated as allow all. To make new MCP servers deny all egress unless domains are explicitly configured, set:

```yaml
config:
  OBOT_SERVER_MCPDEFAULT_DENY_ALL_EGRESS: "true"
```

With the default set to deny all, admins can still allow unrestricted egress for an individual MCP server by using the server's egress control toggle in the MCP server configuration.

## Configure allowed domains

Configure egress domains on the MCP server runtime configuration. This is supported for `npx`, `uvx`, and `containerized` MCP servers.
This can be configured in the UI when creating or editing an MCP server.
See the YAML configuration examples if you manage MCP servers through Git.

### YAML configuration examples

Example `npx` catalog entry:

```yaml
runtime: npx
npxConfig:
  package: "@modelcontextprotocol/server-github"
  egressDomains:
    - api.github.com
    - "*.githubusercontent.com"
```

Example `uvx` configuration:

```yaml
runtime: uvx
uvxConfig:
  package: "example-mcp-server"
  egressDomains:
    - api.example.com
```

Example containerized configuration:

```yaml
runtime: containerized
containerizedConfig:
  image: "ghcr.io/example/mcp-server:latest"
  port: 8080
  path: "/mcp"
  egressDomains:
    - api.example.com
    - "*.example-cdn.com"
```

To block all external egress for a server, set `denyAllEgress: true` and leave `egressDomains` empty:

```yaml
runtime: npx
npxConfig:
  package: "example-mcp-server"
  denyAllEgress: true
```

Domain values must be hostnames only:

- Use `example.com` or a leading wildcard such as `*.example.com`.
- Do not include a protocol, path, or port.
- Do not use IP addresses.
- Internal and reserved names such as `localhost`, `cluster.local`, `*.svc`, and `metadata.google.internal` are rejected.

## How enforcement works

For each supported MCP server, Obot creates an `MCPNetworkPolicy` with the server name, pod selector, allowed domains, and deny-all setting. The Aviatrix provider watches these objects from Obot storage and creates a corresponding `FirewallPolicy` in the MCP runtime namespace.

The generated Aviatrix policy includes:

- A SmartGroup matching the MCP server pods by their `app` label.
- A destination SmartGroup for `0.0.0.0/0`.
- A WebGroup containing the configured egress domains.
- A permit rule for approved domain egress over TCP port `443`.
- A deny rule that blocks other external egress when domains are configured or deny-all is enabled.

:::warning
Domain allowlists are enforced for HTTPS egress on TCP port `443`. Traffic to all other ports will be blocked. Remote MCP servers are not covered by this feature because they are external endpoints rather than Obot-hosted MCP server workloads.
:::

## Verify the setup

Check that Obot installed the provider:

```bash
helm status obot-network-policy-provider -n <obot-namespace>
kubectl get pods -n <obot-namespace> -l app.kubernetes.io/name=aviatrix-network-policy-controller
```

Check that the Aviatrix provider created a `FirewallPolicy`:

```bash
kubectl get firewallpolicies -n <mcp-runtime-namespace>
kubectl describe firewallpolicy -n <mcp-runtime-namespace> <firewall-policy-name>
```

If the expected `FirewallPolicy` does not appear after configuring egress domains on an MCP server, inspect the provider pod logs and confirm that the `FirewallPolicy` CRD is installed:

```bash
kubectl get crd firewallpolicies.networking.aviatrix.com
kubectl logs -n <obot-namespace> -l app.kubernetes.io/name=aviatrix-network-policy-controller
```

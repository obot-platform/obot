# MCP Servers in Kubernetes

This is an overview of how Obot sets up MCP servers in Kubernetes, and how to change some of the configuration values.

## Namespace

Obot will deploy MCP servers into the namespace `{helm-release-name}-mcp`. So if your Helm release name is `obot`,
Obot will deploy servers to the `obot-mcp` namespace.

You can override this namespace and set it to whatever you would like using the Helm value `.mcpNamespace.name`.

## RBAC

In order to set up Deployments, Services, and Secrets, Obot needs a ServiceAccount, Role, and RoleBinding
that give it permissions to do so in the namespace. All of this is included in the Helm chart.

Here is a link to the Role, to view the permissions that Obot will have:
[https://github.com/obot-platform/obot/blob/main/chart/templates/mcp.yaml](https://github.com/obot-platform/obot/blob/main/chart/templates/mcp.yaml)

These permissions are granted only for the namespace where Obot deploys MCP servers.

## K8s objects for each MCP server

Each MCP server will have the following Kubernetes objects created for it:

- A Deployment to run the actual server
- A Service to expose it within the cluster
- Secrets to hold configuration

### Deployment

Obot will set up one Deployment for the MCP server. Most of the configuration for these Deployments is
unchangeable, but some of it can be modified. These are the configuration parameters that **cannot** be changed:

- Replicas: 1
- ImagePullPolicy: `Always`
- SecurityContext: Hardened security settings at both pod and container levels (see [Pod Security Admission](#pod-security-admission) section for details)
  - All capabilities are dropped
  - Privilege escalation is disabled
  - Runs as non-root user (UID 1000)
  - Seccomp profile set to RuntimeDefault
- Environment: sourced from a SecretRef containing the configuration values provided by the user, if any
- Volumes and Volume Mounts: any configuration values from the user that were provided as files, will be mounted from Secrets in this way

The values that are configurable, and how to change them, follow.

#### Configurable Values

- Affinity and Tolerations: can be set using the `.mcpServerDefaults.affinity` and `.mcpServerDefaults.tolerations` in Helm, or via the admin UI if not set in Helm values
- Resources: the default value is a memory request of `400Mi` with no memory limit or CPU requests/limits. This can be set in Helm using the `.mcpServerDefaults.resources` value, or via the Admin UI if not set in Helm values.
- Image: the default value is `ghcr.io/obot-platform/mcp-images/phat:main` and it can be changed by setting the Helm value `.config.OBOT_SERVER_MCPBASE_IMAGE`.

#### A note on Affinity, Tolerations, and Resources

The configuration for affinity, tolerations, and resources applies to all MCP server Deployments across Obot.
It cannot be customized for individual MCP server Deployments.
When this configuration value changes, it will only affect new Deployments (or restarted existing Deployments)
from that point forward. The admin can use the UI to manually apply this configuration change to existing MCP server
Deployments as desired.

### Service

Obot creates one ClusterIP service for each Deployment to expose its MCP server on port 80.

### Network Policy

Obot provides an optional NetworkPolicy to restrict network traffic from MCP server pods for enhanced security. When enabled, this policy limits what MCP servers can access on the network.

#### Configuration

The NetworkPolicy is enabled by default and can be disabled via Helm values:

```yaml
mcpNamespace:
  networkPolicy:
    enabled: false
```

#### Security Model

When enabled, the NetworkPolicy implements the following restrictions:

**Ingress (Incoming Traffic)**
- MCP server pods can **only** receive connections from Obot pods in the main Obot namespace
- All other incoming traffic is blocked

**Egress (Outgoing Traffic)**
MCP server pods can communicate with:
1. **DNS resolution** - UDP/TCP port 53 in the configured DNS namespace (default: `kube-system`)
2. **Obot service** - TCP port 8080 to the main Obot service for callbacks and communication
3. **Public internet** - All public IP addresses for external API calls and services

MCP server pods are **blocked** from accessing:
- Private IP ranges: `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`
- Loopback addresses: `127.0.0.0/8`
- Link-local addresses: `169.254.0.0/16`
- Multicast ranges: `224.0.0.0/4`
- Reserved ranges: `240.0.0.0/4`

:::tip Security Best Practice
Enabling the NetworkPolicy is recommended for production deployments to prevent MCP servers from accessing internal cluster resources or private network services. This helps contain potential security issues if an MCP server is compromised or misconfigured.
:::

:::warning Impact on Internal Services
If your MCP servers need to access internal Kubernetes services or private network resources, you will need to either disable the NetworkPolicy or create additional NetworkPolicy rules to allow specific traffic.
:::

### Pod Security Admission

Obot supports Pod Security Admission (PSA) configuration for the MCP namespace to enforce Kubernetes Pod Security Standards. PSA provides a way to enforce security policies on pods at the namespace level.

#### Configuration

PSA is configured via Helm values with sensible defaults:

```yaml
mcpNamespace:
  podSecurity:
    # Enable or disable PSA labels on the MCP namespace
    enabled: true
    # Enforcement level: privileged, baseline, or restricted
    enforce: restricted
    enforceVersion: latest
    # Audit level: logs policy violations without blocking
    audit: restricted
    auditVersion: latest
    # Warning level: shows warnings for policy violations
    warn: restricted
    warnVersion: latest
```

To disable Pod Security Admission entirely (not recommended), set `enabled: false`:

```yaml
mcpNamespace:
  podSecurity:
    enabled: false
```

#### Pod Security Standards Levels

Kubernetes defines three Pod Security Standards levels:

- **privileged**: Unrestricted policy, providing the widest possible level of permissions
- **baseline**: Minimally restrictive policy which prevents known privilege escalations. Allows the default (minimally specified) Pod configuration.
- **restricted** (default): Heavily restricted policy, following current Pod hardening best practices

#### How PSA Works in Obot

The PSA configuration is applied as labels on the MCP namespace:
- `pod-security.kubernetes.io/enforce`: Blocks pod creation if it violates the policy
- `pod-security.kubernetes.io/audit`: Logs violations to the audit log without blocking
- `pod-security.kubernetes.io/warn`: Returns a warning message to the user for violations

:::tip Security-First Default
Obot uses the **restricted** policy by default, providing the highest level of pod security. This policy follows current Pod hardening best practices and is recommended for production environments. If you need more permissive settings for specific use cases, you can configure the policy to **baseline** or **privileged**.
:::

:::info MCP Pod Security Context
Obot automatically configures MCP pods with secure defaults that comply with the restricted policy:

**Pod-level SecurityContext:**
- `runAsNonRoot: true`
- `runAsUser: 1000`
- `runAsGroup: 1000`
- `fsGroup: 1000`
- `seccompProfile.type: RuntimeDefault`

**Container-level SecurityContext:**
- `allowPrivilegeEscalation: false`
- `runAsNonRoot: true`
- `runAsUser: 1000`
- `runAsGroup: 1000`
- `capabilities.drop: ["ALL"]`
- `seccompProfile.type: RuntimeDefault`

These settings ensure all MCP pods are hardened against common security vulnerabilities and comply with the Kubernetes restricted Pod Security Standard.
:::

#### Environment Variable Configuration

PSA can also be configured via environment variables:

| Environment Variable | Description | Default |
|---------------------|-------------|---------|
| `OBOT_SERVER_MCPPOD_SECURITY_ENABLED` | Enable Pod Security Admission labels on MCP namespace | `true` |
| `OBOT_SERVER_MCPPOD_SECURITY_ENFORCE` | Pod Security Standards level to enforce | `restricted` |
| `OBOT_SERVER_MCPPOD_SECURITY_ENFORCE_VERSION` | Kubernetes version for enforce policy | `latest` |
| `OBOT_SERVER_MCPPOD_SECURITY_AUDIT` | Pod Security Standards level to audit | `restricted` |
| `OBOT_SERVER_MCPPOD_SECURITY_AUDIT_VERSION` | Kubernetes version for audit policy | `latest` |
| `OBOT_SERVER_MCPPOD_SECURITY_WARN` | Pod Security Standards level to warn about | `restricted` |
| `OBOT_SERVER_MCPPOD_SECURITY_WARN_VERSION` | Kubernetes version for warn policy | `latest` |

### Secrets

Obot will create a Secret to contain the user-provided configuration values for the MCP server.
Any configuration values that were marked as files will be in a separate Secret that is mounted in the `/files` directory in the container.

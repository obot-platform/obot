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
- SecurityContext: `allowPrivilegeEscalation` is false, `runAsNonRoot` is true, `runAsUser` is 1000, and `runAsGroup` is 1000
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

:::note
Obot does not create any NetworkPolicies. You can set up your own NetworkPolicies to restrict incoming traffic to the
MCP server Deployments, so that only the main Obot Deployment can connect to them.
We recommend allowing all egress traffic out of the MCP server Deployments.
:::

### Secrets

Obot will create a Secret to contain the user-provided configuration values for the MCP server.
Any configuration values that were marked as files will be in a separate Secret that is mounted in the `/files` directory in the container.

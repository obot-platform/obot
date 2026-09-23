---
title: Image Pull Secrets
---

# Image Pull Secrets

Obot can attach Kubernetes image pull secrets to MCP server Deployments when MCP servers run in Kubernetes. Use this when MCP server images are stored in a private registry such as GHCR, Docker Hub, a self-hosted registry, or Amazon ECR.

There are two ways to configure MCP image pull secrets:

- **Static Helm pull secrets** are configured with the `mcpImagePullSecrets` Helm value. Helm creates the Kubernetes secrets and Obot uses those names for every MCP server Deployment.
- **Managed image pull secrets** are configured in the Obot admin UI. Obot stores the registry configuration, creates an automatically named Kubernetes `kubernetes.io/dockerconfigjson` secret in the MCP namespace, and adds every enabled managed secret to every MCP server Deployment.

Static and managed pull secrets are mutually exclusive. If static `mcpImagePullSecrets` are configured, they remain the source of truth and the managed image pull secrets UI and API are read-only.

## Availability

Managed image pull secrets are available only when both of these are true:

- `OBOT_SERVER_MCPRUNTIME_BACKEND` is `kubernetes` or `k8s` (the Helm chart sets this automatically).
- `OBOT_SERVER_MCPIMAGE_PULL_SECRETS` is unset or empty.

When the feature is unavailable, Obot still shows existing managed image pull secret records, but mutations are blocked and no generated Kubernetes resources are created or updated.

## Deployment Behavior

Obot adds all effective pull secret names to every MCP server Deployment:

- If static pull secrets are configured, only the static names are used.
- If static pull secrets are not configured, every enabled managed image pull secret name is used.
- Managed secret names are sorted and deduplicated before they are written to Deployments.

Changing the effective pull secret list marks existing Kubernetes MCP servers as needing a Kubernetes update. New MCP server Deployments use the current effective list immediately. Existing Deployments are not automatically redeployed.

## Static Helm Pull Secrets

Use static pull secrets when you want credentials to be managed outside Obot, for example by GitOps or an external secret controller.

```yaml
mcpImagePullSecrets:
  - name: my-registry-secret
    registry: ghcr.io
    username: myuser
    password: mytoken
    email: myuser@example.com
```

The Helm chart creates a Docker config JSON secret named `my-registry-secret` in the MCP namespace and sets `OBOT_SERVER_MCPIMAGE_PULL_SECRETS` for the Obot server.

:::note
When `mcpImagePullSecrets` is set, managed image pull secrets are disabled. Remove the Helm value and restart Obot to use managed image pull secrets.
:::

## Managed Basic Registry Credentials

Use a basic credential for registries that accept a username and password or token.

1. Go to **Admin -> Image Pull Secrets**.
2. Click **Create New Secret**.
3. Select **Type = Basic**.
4. Optionally enter a display name.
5. Enter the registry server, username, and password.
6. Save the credential.
7. Test the credential with an image reference from that registry.

The registry server should be only the registry host, for example:

- `ghcr.io`
- `index.docker.io`
- `registry.example.com`
- `registry.example.com:5000`

Do not include a repository path in the registry server.

### Basic Credential Testing

Basic credential tests require an image reference. Obot uses the credentials to request the image manifest through the Docker Registry HTTP API. It does not pull image layers.

Examples:

```text
ghcr.io/example/private-mcp-server:1.0.0
registry.example.com/team/server@sha256:...
```

Use an image that the credential is expected to pull. Testing only proves manifest access for the image you provide.

## Managed Amazon ECR Credentials

Amazon ECR credentials use Kubernetes service account OIDC federation. Obot does not store AWS access keys for ECR. Instead, the Obot controller requests a projected Kubernetes service account token, calls AWS STS, assumes your IAM role, calls ECR `GetAuthorizationToken`, and writes the generated Docker config JSON secret into the MCP namespace.

Create one ECR image pull secret per AWS region. ECR authorization tokens are regional. If your MCP images are in multiple ECR regions, create one managed ECR credential for each region.

### ECR Inputs

In **Admin -> Image Pull Secrets**, click **Add ECR** and configure:

The ECR form shows Obot's issuer URL, service account subject, audience, trust policy, and ECR IAM policy before the credential is saved. Use those values to create the AWS IAM OIDC provider, role trust policy, and permissions policy, then return to Obot with the resulting role ARN. If the Role ARN field is empty or incomplete, Obot shows a placeholder AWS account ID in the trust policy. If Obot cannot show an issuer URL, set **Issuer URL Override** to an externally reachable issuer URL before saving.

Obot uses its own Kubernetes service account subject for ECR refreshes:

```text
system:serviceaccount:<obot-namespace>:<obot-service-account-name>
```

| Field                | Description                                                                                                                                                                            |
| -------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Role ARN             | IAM role that the Obot controller assumes with `sts:AssumeRoleWithWebIdentity`.                                                                                                        |
| Region               | AWS region for the ECR registry, for example `us-east-1`.                                                                                                                              |
| Issuer URL Override  | Optional Kubernetes service account issuer URL. Use this only if Obot cannot discover the issuer or you need a different externally reachable issuer URL.                              |
| Audience             | Optional token audience. Defaults to `sts.amazonaws.com`.                                                                                                                              |
| Refresh Schedule     | Optional cron schedule. Defaults to `0 */6 * * *` (every six hours).                                                                                                                   |
| Display Name         | Optional user-facing name shown in the Obot UI.                                                                                                                                        |

After the credential is saved, Obot continues to show the computed trust policy and ECR IAM policy. Use those values to confirm or update the AWS IAM role.

### AWS IAM Setup for ECR Pull Secrets

These steps use the AWS console. Replace the example values with the values shown by Obot.

Use these example values while following the console steps:

```text
AWS account ID: 123456789012
AWS region: us-east-1
Role name: obot-ecr-pull
Issuer URL: https://issuer.example.com
Subject: system:serviceaccount:obot:obot
Audience: sts.amazonaws.com
```

The issuer URL must be reachable by AWS STS and must publish standard OIDC discovery and JWKS documents. On EKS, this is normally the cluster OIDC issuer URL. For other Kubernetes distributions, verify that the service account issuer is externally reachable and uses an AWS-trusted TLS certificate, or configure an issuer URL override that meets those requirements.

Create the IAM OIDC provider if it does not already exist:

1. In the AWS console, go to **IAM -> Identity providers**.
2. Choose **Add provider**.
3. Select **OpenID Connect**.
4. Enter the **Issuer URL** shown by Obot.
5. Enter `sts.amazonaws.com` as the audience, or the custom audience configured in Obot.
6. Add the provider.

Create the IAM role:

1. Go to **IAM -> Roles**.
2. Choose **Create role**.
3. Choose **Custom trust policy**.
4. Paste the trust policy shown by Obot, or use this template.

AWS IAM condition keys use the issuer URL without the `https://` prefix:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Federated": "arn:aws:iam::123456789012:oidc-provider/issuer.example.com"
      },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "StringEquals": {
          "issuer.example.com:sub": "system:serviceaccount:obot:obot",
          "issuer.example.com:aud": "sts.amazonaws.com"
        }
      }
    }
  ]
}
```

Create the ECR pull policy:

1. Go to **IAM -> Policies**.
2. Choose **Create policy**.
3. Choose **JSON**.
4. Paste the ECR policy shown by Obot, or use this template.

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": "ecr:GetAuthorizationToken",
      "Resource": "*"
    },
    {
      "Effect": "Allow",
      "Action": [
        "ecr:BatchCheckLayerAvailability",
        "ecr:BatchGetImage",
        "ecr:GetDownloadUrlForLayer"
      ],
      "Resource": [
        "arn:aws:ecr:us-east-1:123456789012:repository/*"
      ]
    }
  ]
}
```

Attach the policy:

1. Return to the IAM role.
2. Open the **Permissions** tab.
3. Choose **Add permissions -> Attach policies**.
4. Select the ECR pull policy.
5. Attach it to the role.

Use the role ARN in Obot:

```text
arn:aws:iam::123456789012:role/obot-ecr-pull
```

AWS references:

- [CreateOpenIDConnectProvider](https://docs.aws.amazon.com/IAM/latest/APIReference/API_CreateOpenIDConnectProvider.html)
- [Configuring a role for web identity federation](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_create_for-idp_oidc.html)
- [Using Amazon ECR images with Amazon EKS](https://docs.aws.amazon.com/AmazonECR/latest/userguide/ECR_on_EKS.html)
- [Amazon ECR private repository policies](https://docs.aws.amazon.com/AmazonECR/latest/userguide/repository-policies.html)

### Cross-account ECR Registries

To pull from ECR repositories in another AWS account:

1. Grant the assumed IAM role permission to pull from the repository ARN in that account.
2. If the repository policy restricts principals, allow the IAM role ARN from the Obot account.

Example repository resource for another account:

```json
"arn:aws:ecr:us-east-1:210987654321:repository/private-mcp-server"
```

The ECR credential still covers only one region. Create another ECR credential for a different region.

### ECR Refresh and Test

After the ECR credential is saved, Obot creates these resources:

- A Docker config JSON secret.

The credential becomes `Ready` after the controller successfully writes the Docker config JSON secret. Use **Refresh Now** to request an immediate controller refresh. Use **Test** with a full image reference to verify that the generated secret can access the image manifest.

## Troubleshooting

### Managed Image Pull Secrets Are Unavailable

Check the capability banner in **Admin -> Image Pull Secrets**.

- If the backend is not Kubernetes, set `OBOT_SERVER_MCPRUNTIME_BACKEND=kubernetes` or use the Helm chart default.
- If static pull secrets are configured, remove `mcpImagePullSecrets` from Helm values and restart Obot before using managed image pull secrets.

### Image Pull Secret Is Not on an MCP Deployment

Managed image pull secrets are added to new MCP Deployments immediately. Existing MCP Deployments are marked as needing a Kubernetes update when the effective pull secret list changes. Apply the Kubernetes update from the admin UI for existing deployments.

### Basic Registry Test Fails

Verify:

- The registry server matches the image reference registry exactly.
- The username and password or token can pull the image.
- The image reference includes a tag or digest that exists.
- The registry accepts Docker Registry HTTP API v2 manifest requests.

### ECR Credential Stays Pending

Check the managed secret in the MCP namespace:

```bash
kubectl -n <mcp-namespace> get secret \
  -l obot.ai/managed-by=image-pull-secrets
```

Common causes:

- The IAM OIDC provider was not created for the issuer URL shown by Obot.
- The IAM role trust policy subject does not match the service account subject shown by Obot.
- The trust policy audience is not `sts.amazonaws.com` or does not match the configured audience.
- The issuer URL is not reachable by AWS STS.
- The IAM role policy allows `ecr:GetAuthorizationToken` but not repository pull actions.
- The ECR repository policy blocks the assumed role, especially in cross-account setups.
- The credential is configured for the wrong AWS region.

### Pods Still Show `ImagePullBackOff`

Describe the pod and confirm which secret Kubernetes used:

```bash
kubectl -n <mcp-namespace> describe pod <pod-name>
kubectl -n <mcp-namespace> get deployment <deployment-name> -o yaml
```

Verify that:

- The Deployment has the expected `imagePullSecrets`.
- The generated secret exists in the MCP namespace.
- The secret is type `kubernetes.io/dockerconfigjson`.
- The image registry and region match the configured credential.

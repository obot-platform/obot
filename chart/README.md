# Obot Helm Chart

This Helm chart is used to deploy the Obot server on a Kubernetes cluster. It provides a variety of configuration options to customize the deployment according to your needs.

## Installation

To install the chart with the release name `my-release`:

```Shell
helm install my-release .
```

## Configuration

The following table lists the configurable parameters of the Obot chart and their default values.

### General Configuration

| Parameter                                | Description                                                    | Default                                 |
| ---------------------------------------- | -------------------------------------------------------------- | --------------------------------------- |
| `replicaCount`                           | Number of Obot server instances to run                         | `1`                                     |
| `image.repository`                       | Docker repository for Obot                                     | `ghcr.io/obot-platform/obot-enterprise` |
| `image.tag`                              | Docker tag to pull for Obot                                    | `latest`                                |
| `image.pullPolicy`                       | Kubernetes image pull policy                                   | `IfNotPresent`                          |
| `imagePullSecrets`                       | Kubernetes secrets for pulling private images                  | `[]`                                    |
| `updateStrategy`                         | Update strategy for the deployment (Recreate or RollingUpdate) | `RollingUpdate`                         |
| `service.type`                           | Type of Kubernetes service                                     | `ClusterIP`                             |
| `service.port`                           | Port for the Kubernetes service                                | `80`                                    |
| `ingress.enabled`                        | Enables ingress creation for Obot                              | `false`                                 |
| `ingress.className`                      | Preexisting ingress class to use                               | `~`                                     |
| `config.existingSecret`                  | Name of an existing secret for config                          | `""`                                    |
| `config.awsAccessKeyID`                  | AWS Access Key ID for S3 provider                              | `""`                                    |
| `config.awsRegion`                       | AWS Region for S3 provider                                     | `""`                                    |
| `config.awsSecretAccessKey`              | AWS Secret Access Key for S3 provider                          | `""`                                    |
| `config.baaahThreadiness`                | Threadiness setting for Obot                                   | `"20"`                                  |
| `config.githubAuthToken`                 | GitHub authentication token                                    | `""`                                    |
| `config.obotServerEnableAuthentication`  | Enable authentication for Obot server                          | `true`                                  |
| `config.obotBootstrapToken`              | Bootstrap token for Obot server                                | `""`                                    |
| `config.obotServerAuthAdminEmails`       | Admin emails for Obot server authentication                    | `""`                                    |
| `config.obotServerDSN`                   | Data Source Name for Obot server database                      | `""`                                    |
| `config.obotServerHostname`              | Hostname for Obot server                                       | `""`                                    |
| `config.obotWorkspaceProviderType`       | Workspace provider type (`directory` or `s3`)                  | `"s3"`                                  |
| `config.openaiApiKey`                    | OpenAI API key                                                 | `""`                                    |
| `config.workspaceProviderS3BaseEndpoint` | Base endpoint for S3 provider                                  | `""`                                    |
| `config.workspaceProviderS3Bucket`       | S3 bucket for workspace provider                               | `""`                                    |
| `extraEnv`                               | Additional environment variables to set                        | `{}`                                    |
| `resources`                              | Resource requests and limits for Obot                          | `{}`                                    |
| `serviceAccount.create`                  | Specifies whether a service account should be created          | `false`                                 |
| `serviceAccount.name`                    | Name of the service account to use                             | `""`                                    |

### Configuration Options

[See obot configuration docs](https://docs.obot.ai/configuration/general).

#### Workspace Provider

[See Workspace provider docs](https://docs.obot.ai/configuration/workspace-provider)

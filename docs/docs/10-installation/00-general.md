---
title: General
slug: /installation/general
---

# Overview

## Obot Architecture

Obot is a complete platform for building and running agents. The main components are:

- Obot server
- Postgres database
- Caching directory

Obot stores its data under the `/data` path. If you are not using an external Postgres database, that data will also be under `/data`.

### Production Considerations

For a production setup you will want to use an external Postgres database and configure encryption for your Obot data.

## System requirements

### Minimum

We recommend the following for local testing:

- 2GB of RAM
- 1 CPU core
- 10GB of disk space

### Recommended

- 4GB of RAM
- 2 CPU cores
- 10GB of disk space

## Installation Methods

### Helm

If you would like to install Obot on a Kubernetes cluster, you can use the Helm chart. We are currently working on the Helm chart and have made it available for testing here: [obot-helm](https://charts.obot.ai/)

### Reference Production Deployment

For a production-grade deployment of Obot, we should have the following infrastructure:
- A Postgres database
- An S3-compatible bucket for workspace storage
- A Cloud KMS provider for encrypting sensitive/secret information

The helm chart has mostly sane production default settings, but you will need to configure the database, workspace provider, and encryption provider.

```yaml
# Enable ingress or use a service of type loadbalancer to expose Obot
ingress:
  enabled: true
  hosts:
    - <your obot hostname>

# This can be turned off because we are persisting data externally in postgres and S3
persistence:
  enabled: false

# In this example, we will be using S3 and AWS KMS for encryption
config:
  # this should have IAM permissions for S3 and KMS
  AWS_ACCESS_KEY_ID: <access key>
  AWS_SECRET_ACCESS_KEY: <secret key>
  AWS_REGION: <aws region>

  # This should be set to avoid ratelimiting certain actions that interact with github, such as catalogs
  GITHUB_AUTH_TOKEN: <PAT from github>

  # Enable encryption
  OBOT_SERVER_ENCRYPTION_PROVIDER: aws
  OBOT_AWS_KMS_KEY_ARN: <your kms arn>

  # Enable S3 workspace provider
  OBOT_WORKSPACE_PROVIDER_TYPE: s3
  WORKSPACE_PROVIDER_S3_BUCKET: <s3 bucket name>

  # optional - this will be generated automatically if you do not set it
  OBOT_BOOTSTRAP_TOKEN: <some random value>

  # Point this to your postgres database
  OBOT_SERVER_DSN: postgres://<user>:<pass>@<host>/<db>
  
  OBOT_SERVER_HOSTNAME: <your obot hostname>
  OPENAI_API_KEY: <openai api key>
```


## Next Steps

- [Configure Authentication](/configuration/auth-providers)

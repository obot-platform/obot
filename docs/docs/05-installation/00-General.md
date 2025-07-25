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
- 40GB of disk space

Along with external Postgres and S3 storage for production use cases.

## Installation Methods

### Helm

If you would like to install Obot on a Kubernetes cluster, you can use the Helm chart. We are currently working on the Helm chart and have made it available for testing here: [obot-helm](https://charts.obot.ai/)

## Next Steps

- [Configure Authentication](/configuration/auth-providers)

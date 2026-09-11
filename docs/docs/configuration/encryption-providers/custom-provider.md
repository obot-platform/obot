# Custom Encryption Provider

This guide explains how to set up custom encryption for Obot using a local encryption key.

## Overview

The custom encryption provider uses AES-GCM encryption with a secret key that you provide. This is useful when:
- You want encryption at rest but don't have access to cloud KMS services
- You're running Obot in air-gapped or on-premises environments
- You want a simpler encryption setup without external dependencies

> **Note**: Unlike AWS KMS, Google Cloud KMS, or Azure Key Vault, the custom provider stores the encryption key locally. You are responsible for securing and backing up this key.

## Prerequisites

- OpenSSL or similar tool to generate a secure random key

## Configuration Steps

### 1. Generate an Encryption Key

Generate a secure 32-byte random key and encode it in base64:

```bash
openssl rand -base64 32
```

This will output a string like:
```
Kj8fH2lP9mQ4nR6tV8xZ0bC3dE5gF7hI9jK1lM3nO5p=
```

> **Security Warning**: Keep this key secret and secure. Anyone with access to this key can decrypt your data. Store it in a secure location such as a password manager or secrets management system.

### 2. Configure Your Deployment

#### Helm

Set the provider under `config` and the key you generated in step 1 under `secret` in your values file:

```yaml
config:
  OBOT_SERVER_ENCRYPTION_PROVIDER: custom
secret:
  OBOT_SERVER_ENCRYPTION_KEY: "<your-base64-key>"
```

The Helm chart uses this key to enable encryption for the [supported resources](./overview.md#encrypted-resources-and-fields).

#### Standalone Server or Docker

Create an `EncryptionConfiguration` file with your generated key:

```yaml
apiVersion: apiserver.config.k8s.io/v1
kind: EncryptionConfiguration
resources:
  - resources:
      - credentials.obot.obot.ai
      - users.obot.obot.ai
      - identities.obot.obot.ai
      - mcpoauthtokens.obot.obot.ai
      - mcpoauthpendingstates.obot.obot.ai
      - mcpauditlogs.obot.obot.ai
      - llmauditlogs.obot.obot.ai
      - policyviolations.obot.obot.ai
      - properties.obot.obot.ai
    providers:
      - aesgcm:
          keys:
            - name: key0
              secret: "<your-base64-key>"
      - identity: {}
```

Make the file available to the Obot process, for example by mounting it read-only at `/config/encryption.yaml` in Docker. Restrict access because the file contains your encryption key. Set:

```text
OBOT_SERVER_ENCRYPTION_PROVIDER=custom
OBOT_SERVER_ENCRYPTION_CONFIG_FILE=/config/encryption.yaml
```

`OBOT_SERVER_ENCRYPTION_KEY` alone is insufficient for a standalone server; the chart performs the file-generation step.

For existing data, see [enabling encryption on an existing installation](./overview.md#enabling-encryption-on-an-existing-installation).

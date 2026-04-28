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

### Obot environment variables

Make sure the following environment variables are set on Obot when you run it:

- `OBOT_SERVER_ENCRYPTION_PROVIDER="custom"`
- `OBOT_SERVER_ENCRYPTION_KEY="<your-base64-key>"`

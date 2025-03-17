# AWS KMS

This guide explains how to set up AWS KMS encryption for Obot.

### Prerequisites

- An AWS KMS key with Key Type `Symmetric` and Key Usage `Encrypt and Decrypt`
- The proper permissions and credentials to access it

### Obot environment variables

Make sure the following environment variables are set on Obot when you run it:

- `OBOT_SERVER_ENCRYPTION_PROVIDER=aws`
- `OBOT_AWS_KMS_KEY_ARN=<your key ARN>`

### AWS credentials

The credentials can be provided to Obot either via the standard environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN`, `AWS_REGION`) or through some sort of metadata server setup with EC2 or IRSA in Kubernetes.

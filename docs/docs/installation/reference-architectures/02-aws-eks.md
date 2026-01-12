# Amazon EKS

Deploying Obot to Amazon Elastic Kubernetes Service follows the standard Helm workflow. As a prerequisite, you'll need the following resources set up in your AWS environment:

* AWS account
* VPC with subnets
* Amazon RDS instance running PostgreSQL 17+ with the pgvector extension enabled
* VPC Security Groups configured to allow connectivity between your EKS cluster and RDS instance
* Private Amazon S3 bucket for workspace data
* (Optional) AWS KMS key for encrypting sensitive information
* (Optional) IAM role and policy for service account (IRSA) if you're using AWS services like KMS for encryption
* kubectl and Helm installed and configured to connect to your EKS cluster
* EKS cluster with at least 2 CPU cores and 4GB of RAM per node. Production workloads may require more. The cluster should have IAM roles for service accounts (IRSA) configured if you're using AWS services like KMS for encryption.

If you plan on using AWS KMS, here is some example terraform that creates the key and the necessary IAM policies:

```hcl
resource "aws_kms_key" "this" {
  description             = "Obot credentials encryption key"
  deletion_window_in_days = 10
  enable_key_rotation     = true
}

resource "aws_kms_alias" "this" {
  name          = "alias/obot-credentials"
  target_key_id = aws_kms_key.this.key_id
}

data "aws_iam_policy_document" "obot_kms" {
  statement {
    effect = "Allow"
    actions = [
      "kms:Decrypt",
      "kms:Encrypt",
      "kms:GenerateDataKey",
      "kms:DescribeKey"
    ]
    resources = [aws_kms_key.this.arn]
  }
}

resource "aws_iam_policy" "obot_kms" {
  name        = "obot-kms-policy"
  description = "Policy for Obot to use KMS for encryption"
  policy      = data.aws_iam_policy_document.obot_kms.json
}

# Attach this policy to the IAM role used by the Obot service account
resource "aws_iam_role_policy_attachment" "obot_kms" {
  role       = "<name of the IAM role for obot service account>"
  policy_arn = aws_iam_policy.obot_kms.arn
}
```

More information on the AWS KMS setup can be found [here](/configuration/encryption-providers/aws-kms/).

Once you have these resources set up, install the Obot helm chart with:

```bash
helm repo add obot https://charts.obot.ai
helm install obot obot/obot -f <path to your values.yaml>
```

Here is an example `values.yaml` file for deploying Obot on EKS:

```yaml
# These settings are required for EKS when using the AWS Load Balancer Controller.
service:
  type: NodePort
ingress:
  enabled: true
  className: alb
  annotations:
    alb.ingress.kubernetes.io/scheme: internet-facing
    alb.ingress.kubernetes.io/target-type: ip
  hosts:
    - host: <your hostname>

serviceAccount:
  # This is important for configuring IAM roles for service accounts (IRSA), which we use for AWS KMS access
  create: true
  name: "<name of the service account to be created and used by obot>"
  annotations:
    eks.amazonaws.com/role-arn: "<arn of the IAM role to be assumed by obot>"

config:
  # configures encryption with AWS KMS. optional, but recommended for production
  OBOT_SERVER_ENCRYPTION_PROVIDER: "aws"
  OBOT_AWS_KMS_KEY_ARN: "arn:aws:kms:<region>:<account-id>:key/<key-id>"

  # database configuration for external db
  OBOT_SERVER_DSN: "postgresql://<db user>:<db password>@<db host>:<db port>/<db name>?sslmode=<ssl mode>"

  # Enable authentication
  OBOT_SERVER_ENABLE_AUTHENTICATION: true
  OBOT_BOOTSTRAP_TOKEN: "<bootstrap password>"

  # Optionally Preseed admin and owner users
  OBOT_SERVER_AUTH_ADMIN_EMAILS: "<comma separated list of admin emails>"
  OBOT_SERVER_AUTH_OWNER_EMAILS: "<comma separated list of owner emails>"

  # Configure S3 for workspace storage
  OBOT_WORKSPACE_PROVIDER_TYPE: "s3"
  WORKSPACE_PROVIDER_S3_BUCKET: "<your bucket name>"
  AWS_REGION: "<your aws region>"

  # Optionally configure model providers
  OPENAI_API_KEY: "<your openai api key>"
```

With the default configuration on EKS, this will set up ingress to expose Obot through an Application Load Balancer using the AWS Load Balancer Controller. Make sure you have the AWS Load Balancer Controller installed in your cluster. You should also consider adding TLS termination to your ALB for secure HTTPS access.

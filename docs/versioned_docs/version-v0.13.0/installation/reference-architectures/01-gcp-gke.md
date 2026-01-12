# Google Cloud GKE

Deploying Obot to Google Kubernetes Engine follows the standard Helm workflow. As a prerequisite, you'll need the following resources set up in your Google Cloud environment:

* GCP project
* VPC Network
* Google Cloud SQL instance running PostgreSQL 17+ with the pgvector extension enabled
* VPC Network Peering between your VPC Network and the Cloud SQL instance
* Private Google Cloud Storage bucket for workspace data
* (Optional) Google Cloud KMS key for encrypting sensitive information
* (Optional) IAM role with necessary permissions if you're using Google Cloud KMS for encryption
* kubectl and Helm installed and configured to connect to your GKE cluster
* GKE cluster with at least 2 CPU cores and 4GB of RAM per node. Production workloads may require more. The cluster should have workload identity configured if you're using Google Cloud services like Cloud KMS for encryption.

To use the load balancer that Google will create when you deploy the chart, you will need to create your own BackendConfig. Here is an example of a terraform resource that can create the proper BackendConfig:

```hcl
resource "kubernetes_manifest" "obot-backendconfig" {
  manifest = {
    "apiVersion" = "cloud.google.com/v1"
    "kind"       = "BackendConfig"
    "metadata" = {
      "name"      = "<name of your backend config>"
      "namespace" = "<namespace where obot is installed>"
    }
    "spec" = {
      "timeoutSec" = 600
      "healthCheck" = {
        "checkIntervalSec"   = 10
        "timeoutSec"         = 5
        "healthyThreshold"   = 1
        "unhealthyThreshold" = 3
        "type"               = "HTTP"
        "requestPath"        = "/api/healthz"
        "port"               = 8080
      }
    }
  }
}
```

If you plan on using Google Cloud KMS, here is some example terraform that creates the keyring, key, and the necessary IAM bindings:

```hcl
resource "google_kms_key_ring" "this" {
  name     = "obot-credentials"
  location = "us-central1"
  project  = "<your google project id>"
}

resource "google_kms_crypto_key" "this" {
  name     = "obot-credentials"
  key_ring = google_kms_key_ring.this.id
  purpose  = "ENCRYPT_DECRYPT"
}

resource "google_kms_crypto_key_iam_binding" "this" {
  crypto_key_id = google_kms_crypto_key.this.id
  role          = "roles/cloudkms.cryptoKeyEncrypterDecrypter"

  members = [
    "principal://iam.googleapis.com/projects/<your google project number>/locations/global/workloadIdentityPools/<your google project id>.svc.id.goog/subject/ns/<name of the kubernets namespace where obot is deployed>/sa/<name of the service account used by obot>",
  ]
}
```

More information on the Google Cloud KMS setup can be found [here](/configuration/encryption-providers/google-cloud-kms).


Once you have these resources set up, install the Obot helm chart with:

```bash
helm repo add obot https://charts.obot.ai
helm install obot obot/obot -f <path to your values.yaml>
```

Here is an example `values.yaml` file for deploying Obot on GKE:

```yaml
# These settings are required for GKE when using the default Ingress controller.
service:
  type: NodePort
  annotations:
    cloud.google.com/neg: '{"ingress": true}'
    cloud.google.com/backend-config: '{"ports":{"80":"<name of your backend config>"}}'
ingress:
  enabled: true
  hosts:
    - host: <your hostname>

serviceAccount:
  # This is important for configuring Google Workload Identity, which we use for Google Cloud KMS access
  create: true
  name: "<name of the service account to be created and used by obot>"

config:
  # configures encryption with Google Cloud KMS. optional, but recommended for production
  OBOT_SERVER_ENCRYPTION_PROVIDER: "GCP"
  OBOT_GCP_KMS_KEY_URI: "projects/<your project>/locations/<your location>/keyRings/<your key ring>/cryptoKeys/<your key>"

  # database configuration for external db
  OBOT_SERVER_DSN: "postgresql://<db user>:<db password>@<db host>:<db port>/<db name>?sslmode=<ssl mode>"

  # Enable authentication
  OBOT_SERVER_ENABLE_AUTHENTICATION: true
  OBOT_BOOTSTRAP_TOKEN: "<bootstrap password>"

  # Optionally Preseed admin and owner users
  OBOT_SERVER_AUTH_ADMIN_EMAILS: "<comma separated list of admin emails>"
  OBOT_SERVER_AUTH_OWNER_EMAILS: "<comma separated list of owner emails>"

  # Configure GCS for workspace storage
  # Set to s3 and use the s3 compatible settings below
  OBOT_WORKSPACE_PROVIDER_TYPE: "s3"
  WORKSPACE_PROVIDER_S3_BASE_ENDPOINT: "https://storage.googleapis.com"
  WORKSPACE_PROVIDER_S3_BUCKET: "<your bucket name>"
  AWS_REGION: "auto"
  AWS_ACCESS_KEY_ID: "<your google api key id>"
  AWS_SECRET_ACCESS_KEY: "<your google api key secret>"

  # Optionally configure model providers
  OPENAI_API_KEY: "<your openai api key>"
```

With the default configuration on GKE, this will set up ingress to expose Obot through a load balancer. You should also consider adding TLS termination to your load balancer for secure HTTPS access.

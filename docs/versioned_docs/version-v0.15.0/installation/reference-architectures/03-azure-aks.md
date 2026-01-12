# Azure AKS

Deploying Obot to Azure Kubernetes Service follows the standard Helm workflow. As a prerequisite, you'll need the following resources set up in your Azure environment:

* Azure subscription
* Virtual Network with subnets
* Azure Database for PostgreSQL running PostgreSQL 17+ with the pgvector extension enabled
* Network Security Groups configured to allow connectivity between your AKS cluster and PostgreSQL instance
* Private Azure Storage Account with Blob Storage container for workspace data
* (Optional) Azure Key Vault key for encrypting sensitive information
* (Optional) Managed Identity with necessary permissions if you're using Azure Key Vault for encryption
* kubectl and Helm installed and configured to connect to your AKS cluster
* AKS cluster with at least 2 CPU cores and 4GB of RAM per node. Production workloads may require more. The cluster should have Microsoft Entra Workload ID configured if you're using Azure services like Key Vault for encryption.

If you plan on using Azure Key Vault, here is some example terraform that creates the key vault, key, and the necessary access policies:

```hcl
resource "azurerm_key_vault" "this" {
  name                = "obot-credentials-kv"
  location            = azurerm_resource_group.this.location
  resource_group_name = azurerm_resource_group.this.name
  tenant_id           = data.azurerm_client_config.current.tenant_id
  sku_name            = "standard"

  enabled_for_disk_encryption = true
  purge_protection_enabled    = true
}

resource "azurerm_key_vault_key" "this" {
  name         = "obot-credentials"
  key_vault_id = azurerm_key_vault.this.id
  key_type     = "RSA"
  key_size     = 2048

  key_opts = [
    "decrypt",
    "encrypt",
    "sign",
    "unwrapKey",
    "verify",
    "wrapKey",
  ]
}

resource "azurerm_user_assigned_identity" "obot" {
  name                = "obot-identity"
  location            = azurerm_resource_group.this.location
  resource_group_name = azurerm_resource_group.this.name
}

resource "azurerm_key_vault_access_policy" "obot" {
  key_vault_id = azurerm_key_vault.this.id
  tenant_id    = data.azurerm_client_config.current.tenant_id
  object_id    = azurerm_user_assigned_identity.obot.principal_id

  key_permissions = [
    "Get",
    "Decrypt",
    "Encrypt",
    "UnwrapKey",
    "WrapKey",
  ]
}

# Configure federated identity credential for workload identity
resource "azurerm_federated_identity_credential" "obot" {
  name                = "obot-federated-identity"
  resource_group_name = azurerm_resource_group.this.name
  parent_id           = azurerm_user_assigned_identity.obot.id
  audience            = ["api://AzureADTokenExchange"]
  issuer              = azurerm_kubernetes_cluster.this.oidc_issuer_url
  subject             = "system:serviceaccount:<namespace where obot is deployed>:<name of the service account used by obot>"
}
```

More information on the Azure Key Vault setup can be found [here](/configuration/encryption-providers/azure-key-vault).

Once you have these resources set up, install the Obot helm chart with:

```bash
helm repo add obot https://charts.obot.ai
helm install obot obot/obot -f <path to your values.yaml>
```

Here is an example `values.yaml` file for deploying Obot on AKS:

```yaml
# These settings are required for AKS when using the Azure Application Gateway Ingress Controller or NGINX Ingress Controller.
service:
  type: ClusterIP
ingress:
  enabled: true
  className: azure-application-gateway  # or nginx, depending on your ingress controller
  annotations:
    appgw.ingress.kubernetes.io/backend-path-prefix: "/"
  hosts:
    - host: <your hostname>

serviceAccount:
  # This is important for configuring Azure Workload Identity, which we use for Azure Key Vault access
  create: true
  name: "<name of the service account to be created and used by obot>"
  annotations:
    azure.workload.identity/client-id: "<client id of the managed identity>"

config:
  # configures encryption with Azure Key Vault. optional, but recommended for production
  OBOT_SERVER_ENCRYPTION_PROVIDER: "azure"
  OBOT_AZURE_KEY_VAULT_NAME: "<your-keyvault-name>"
  OBOT_AZURE_KEY_NAME: "<your-key-name>"
  OBOT_AZURE_KEY_VERSION: "<your-key-version>"

  # database configuration for external db
  OBOT_SERVER_DSN: "postgresql://<db user>:<db password>@<db host>:<db port>/<db name>?sslmode=<ssl mode>"

  # Enable authentication
  OBOT_SERVER_ENABLE_AUTHENTICATION: true
  OBOT_BOOTSTRAP_TOKEN: "<bootstrap password>"

  # Optionally Preseed admin and owner users
  OBOT_SERVER_AUTH_ADMIN_EMAILS: "<comma separated list of admin emails>"
  OBOT_SERVER_AUTH_OWNER_EMAILS: "<comma separated list of owner emails>"

  # Configure Azure Blob Storage for workspace storage
  OBOT_WORKSPACE_PROVIDER_TYPE: "azure"
  WORKSPACE_PROVIDER_AZURE_CONTAINER: "<your container name>"
  WORKSPACE_PROVIDER_AZURE_CONNECTION_STRING: "<your storage account connection string>"

  # Optionally configure model providers
  OPENAI_API_KEY: "<your openai api key>"
```

With the default configuration on AKS, this will set up ingress to expose Obot through an Application Gateway or NGINX Ingress Controller. Make sure you have the appropriate ingress controller installed in your cluster. You should also consider adding TLS termination to your ingress for secure HTTPS access.

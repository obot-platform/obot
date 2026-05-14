# Azure Key Vault

This guide explains how to set up an Azure VM with a managed identity that can access an Azure Key Vault for encryption operations.

### Prerequisites

- Azure CLI installed and logged in (`az login`)
- A subscription with permissions to create resources

### 1. Create Resource Group

```bash
# Create a resource group
az group create \
  --name your-resource-group \
  --location eastus
```

### 2. Create Key Vault and Key

```bash
# Create a Key Vault with RBAC authorization
az keyvault create \
  --name your-keyvault-name \
  --resource-group your-resource-group \
  --location eastus \
  --enable-rbac-authorization true

# Create an RSA key in the Key Vault
az keyvault key create \
  --name your-key-name \
  --vault-name your-keyvault-name \
  --size 4096 \
  --kty RSA
```

### 3. Create VM with System-Assigned Managed Identity

```bash
# Create VM with system-assigned managed identity
az vm create \
  --resource-group your-resource-group \
  --name your-vm-name \
  --image Ubuntu2204 \
  --admin-username azureuser \
  --generate-ssh-keys \
  --size Standard_B4ms \
  --assign-identity \
  --public-ip-sku Standard
```

### 4. Grant Key Vault Permissions

```bash
# Get the VM's managed identity object ID
IDENTITY_OBJECT_ID=$(az vm identity show \
  --resource-group your-resource-group \
  --name your-vm-name \
  --query principalId \
  --output tsv)

# Get the Key Vault resource ID
KEYVAULT_ID=$(az keyvault show \
  --name your-keyvault-name \
  --query id \
  --output tsv)

# Assign Key Vault Crypto User role
az role assignment create \
  --role "Key Vault Crypto User" \
  --assignee-object-id $IDENTITY_OBJECT_ID \
  --scope $KEYVAULT_ID
```

### 5. Get the Key Version

1. Get the key version:

```bash
az keyvault key show \
  --name your-key-name \
  --vault-name your-keyvault-name \
  --query key.kid \
  --output tsv
```

The key version is the last segment of the key ID (after the last slash).

### 6. Run Obot using Docker

When running Obot with Docker, make sure you include the following environment variables:

- `OBOT_SERVER_ENCRYPTION_PROVIDER=azure`
- `OBOT_AZURE_KEY_VAULT_NAME=your-keyvault-name`
- `OBOT_AZURE_KEY_NAME=your-key-name`
- `OBOT_AZURE_KEY_VERSION=your-key-version`

When running `docker run`, be sure to include `--add-host=metadata.azure.com:169.254.169.254` to ensure the VM's metadata service is accessible.

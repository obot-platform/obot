# Google Cloud KMS

This guide explains how to set up Google Cloud KMS encryption for Obot.

### Prerequisites

- A Google Cloud KMS key ring with a `Symmetric encrypt/decrypt` key
- The proper permissions and credentials to access it

### Obot environment variables

Make sure the following environment variables are set on Obot when you run it:

- `OBOT_SERVER_ENCRYPTION_PROVIDER=gcp`
- `OBOT_GCP_KMS_KEY_URI=projects/<your project>/locations/<your location>/keyRings/<your key ring>/cryptoKeys/<your key>`

### Google Cloud credentials

Some form of credentials is required for Obot to authenticate with Google Cloud for encryption and decryption.
It will look for credentials in the following formats, in this order, until it finds one:

1. A JSON file pointed to by the `GOOGLE_APPLICATION_CREDENTIALS` environment variable
2. A JSON file located at `$HOME/.config/gcloud/application_default_credentials.json`
3. If running on GCE, it will automatically attempt to fetch credentials from the metadata server

If using a JSON file (one of the first two options), the file must be in one of the following two formats:

1. The `credentials.json` format (see [here](https://developers.google.com/workspace/guides/create-credentials#create_credentials_for_a_service_account))
2. A file containing some of the following fields:
   ```
    // Service Account fields
	"client_email"`
	"private_key_id"`
	"private_key"`
	"auth_uri"`
	"token_uri"`
	"project_id"`
	"universe_domain"`

	// User Credential fields
	// (These typically come from gcloud auth.)
	"client_secret"`
	"client_id"`
	"refresh_token"`
   ```
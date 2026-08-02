package encryption

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/obot-platform/obot/logger"
	"k8s.io/apiserver/pkg/server/options/encryptionconfig"
)

var log = logger.Package()

type Options struct {
	AWSKMSKeyARN          string `usage:"The ARN of the AWS KMS key to use for encrypting credential storage. Only used with the AWS encryption provider." env:"OBOT_AWS_KMS_KEY_ARN" name:"aws-kms-key-arn"`
	GCPKMSKeyURI          string `usage:"The URI of the Google Cloud KMS key to use for encrypting credential storage. Only used with the GCP encryption provider." env:"OBOT_GCP_KMS_KEY_URI" name:"gcp-kms-key-uri"`
	AzureKeyVaultName     string `usage:"The name of the Azure Key Vault to use for encrypting credential storage. Only used with the Azure encryption provider." env:"OBOT_AZURE_KEY_VAULT_NAME" name:"azure-key-vault-name"`
	AzureKeyName          string `usage:"The name of the Azure Key Vault key to use for encrypting credential storage. Only used with the Azure encryption provider." env:"OBOT_AZURE_KEY_NAME" name:"azure-key-vault-key-name"`
	AzureKeyVersion       string `usage:"The version of the Azure Key Vault key to use for encrypting credential storage. Only used with the Azure encryption provider." env:"OBOT_AZURE_KEY_VERSION" name:"azure-key-vault-key-version"`
	EncryptionProvider    string `usage:"The encryption provider to use. Options are AWS, GCP, None, or Custom. Default is None." default:"None"`
	EncryptionConfigFile  string `usage:"The path to the encryption configuration file. Only used with the Custom encryption provider."`
	EncryptionKey         string `usage:"A base64-encoded 32-byte AES key used to build the Custom encryption configuration when no file is provided."`
	EncryptionKeyID       string `usage:"The key ID used for new ciphertext when EncryptionKey is configured." default:"key0"`
	EncryptionRetiredKeys string `usage:"A JSON object mapping retired key IDs to base64-encoded 32-byte AES keys used for decrypt-only rotation support."`
}

func (o *Options) Validate() error {
	switch strings.ToLower(o.EncryptionProvider) {
	case "aws":
		if o.AWSKMSKeyARN == "" {
			return fmt.Errorf("missing AWS KMS key ARN")
		}
		o.EncryptionConfigFile = "/aws-encryption.yaml"
	case "gcp":
		if o.GCPKMSKeyURI == "" {
			return fmt.Errorf("missing GCP KMS key URI")
		}
		o.EncryptionConfigFile = "/gcp-encryption.yaml"
	case "azure":
		if o.AzureKeyVaultName == "" || o.AzureKeyName == "" || o.AzureKeyVersion == "" {
			return fmt.Errorf("missing Azure Key Vault configuration")
		}
		o.EncryptionConfigFile = "/azure-encryption.yaml"
	case "custom":
		if o.EncryptionConfigFile == "" && o.EncryptionKey == "" {
			return fmt.Errorf("missing custom encryption config file or key")
		}
		if o.EncryptionConfigFile != "" && o.EncryptionKey != "" {
			return fmt.Errorf("custom encryption config file and key are mutually exclusive")
		}
	case "none", "":
		if o.EncryptionConfigFile != "" {
			return fmt.Errorf("encryption config file provided but encryption provider is set to 'none', use 'custom' encryption provider instead")
		}
	default:
		return fmt.Errorf("invalid encryption provider %s", o.EncryptionProvider)
	}

	return nil
}

func Init(ctx context.Context, opts Options) (*encryptionconfig.EncryptionConfiguration, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	// Set up encryption provider
	switch strings.ToLower(opts.EncryptionProvider) {
	case "aws":
		if err := setUpAWSKMS(ctx, opts.AWSKMSKeyARN); err != nil {
			return nil, fmt.Errorf("failed to setup AWS KMS: %w", err)
		}
	case "gcp":
		if err := setUpGoogleKMS(ctx, opts.GCPKMSKeyURI); err != nil {
			return nil, fmt.Errorf("failed to setup Google Cloud KMS: %w", err)
		}
	case "azure":
		if err := setUpAzureKeyVault(ctx, opts.AzureKeyVaultName, opts.AzureKeyName, opts.AzureKeyVersion); err != nil {
			return nil, fmt.Errorf("failed to setup Azure Key Vault: %w", err)
		}
	}
	if strings.EqualFold(opts.EncryptionProvider, "custom") && opts.EncryptionKey != "" {
		return loadManagedAESConfig(ctx, opts.EncryptionKeyID, opts.EncryptionKey, opts.EncryptionRetiredKeys)
	}

	if opts.EncryptionConfigFile != "" {
		log.Infof("Encryption: Using encryption config file: %s", opts.EncryptionConfigFile)
		ec, err := encryptionconfig.LoadEncryptionConfig(ctx, opts.EncryptionConfigFile, false, "obot")
		return ec, err
	}

	log.Warnf("Encryption: No encryption config file provided, using unencrypted storage")
	return nil, nil
}

func loadManagedAESConfig(ctx context.Context, activeKeyID, encodedKey, encodedRetiredKeys string) (*encryptionconfig.EncryptionConfiguration, error) {
	if activeKeyID == "" {
		activeKeyID = "key0"
	}
	if err := validateManagedKeyID(activeKeyID); err != nil {
		return nil, err
	}
	if err := validateManagedKey(encodedKey); err != nil {
		return nil, fmt.Errorf("custom encryption key %w", err)
	}
	retiredKeys := map[string]string{}
	if strings.TrimSpace(encodedRetiredKeys) != "" {
		if err := json.Unmarshal([]byte(encodedRetiredKeys), &retiredKeys); err != nil {
			return nil, fmt.Errorf("custom retired encryption keys must be a JSON object: %w", err)
		}
	}
	retiredKeyIDs := make([]string, 0, len(retiredKeys))
	for keyID, key := range retiredKeys {
		if keyID == activeKeyID {
			return nil, fmt.Errorf("custom retired encryption keys must not include active key ID %s", activeKeyID)
		}
		if err := validateManagedKeyID(keyID); err != nil {
			return nil, err
		}
		if err := validateManagedKey(key); err != nil {
			return nil, fmt.Errorf("custom retired encryption key %s %w", keyID, err)
		}
		retiredKeyIDs = append(retiredKeyIDs, keyID)
	}
	sort.Strings(retiredKeyIDs)
	file, err := os.CreateTemp("", "obot-encryption-*.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to create custom encryption config: %w", err)
	}
	path := file.Name()
	defer os.Remove(path)
	var keys strings.Builder
	fmt.Fprintf(&keys, "            - name: %s\n              secret: %q\n", activeKeyID, encodedKey)
	for _, keyID := range retiredKeyIDs {
		fmt.Fprintf(&keys, "            - name: %s\n              secret: %q\n", keyID, retiredKeys[keyID])
	}
	config := fmt.Sprintf(`apiVersion: apiserver.config.k8s.io/v1
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
%s
      - identity: {}
`, strings.TrimSuffix(keys.String(), "\n"))
	if _, err := io.WriteString(file, config); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("failed to write custom encryption config: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("failed to close custom encryption config: %w", err)
	}
	loaded, err := encryptionconfig.LoadEncryptionConfig(ctx, path, false, "obot")
	if err != nil {
		return nil, fmt.Errorf("failed to load custom encryption config: %w", err)
	}
	return loaded, nil
}

func validateManagedKeyID(keyID string) error {
	if len(keyID) > 64 || keyID == "" {
		return fmt.Errorf("custom encryption key ID must be 1-64 URL-safe characters")
	}
	for _, char := range keyID {
		urlSafe := (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '.' || char == '_' || char == '-'
		if !urlSafe {
			return fmt.Errorf("custom encryption key ID must be 1-64 URL-safe characters")
		}
	}
	return nil
}

func validateManagedKey(encodedKey string) error {
	key, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil || len(key) != 32 {
		return fmt.Errorf("must be base64-encoded 32-byte data")
	}
	return nil
}

func setUpAzureKeyVault(ctx context.Context, keyvaultName, keyName, keyVersion string) error {
	if keyvaultName == "" || keyName == "" || keyVersion == "" {
		return fmt.Errorf("missing Azure Key Vault configuration")
	}

	if err := os.WriteFile("/tmp/azure.json", []byte(`{"useManagedIdentityExtension": true}`), 0600); err != nil {
		return fmt.Errorf("failed to write Azure config file: %w", err)
	}

	cmd := exec.CommandContext(ctx,
		"azure-encryption-provider",
		"--config-file-path=/tmp/azure.json",
		"--listen-addr=unix:///tmp/azure-cred-socket.sock",
		"--keyvault-name="+keyvaultName,
		"--key-name="+keyName,
		"--key-version="+keyVersion,
		"--healthz-port=22223")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		err := cmd.Wait()
		select {
		case <-ctx.Done():
			// ignore error if we are shutting down
		default:
			log.Fatalf("azure-encryption-provider exited: %v", err)
		}
	}()

	return nil
}

func setUpGoogleKMS(ctx context.Context, kmsKeyURI string) error {
	if kmsKeyURI == "" {
		return fmt.Errorf("missing GCP KMS key URI")
	}

	cmd := exec.CommandContext(ctx,
		"gcp-encryption-provider",
		"--logtostderr",
		"--path-to-unix-socket=/tmp/gcp-cred-socket.sock",
		"--healthz-port=22222",
		"--key-uri="+kmsKeyURI)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		err := cmd.Wait()
		select {
		case <-ctx.Done():
			// ignore error if we are shutting down
		default:
			log.Fatalf("gcp-encryption-provider exited: %v", err)
		}
	}()

	// Wait for the encryption provider to be ready
	var successful bool
	for range 5 {
		time.Sleep(time.Second)

		resp, err := http.Get("http://localhost:22222/healthz")
		if err == nil {
			if resp.StatusCode == http.StatusOK {
				successful = true
				break
			}
			body, _ := io.ReadAll(resp.Body)
			log.Errorf("gcp-encryption-provider health check failed: %s", body)
			_ = resp.Body.Close()
			return fmt.Errorf("gcp-encryption-provider health check failed: %d", resp.StatusCode)
		}
	}

	if !successful {
		return fmt.Errorf("timed out waiting for gcp-encryption-provider to be ready")
	}

	return nil
}

func setUpAWSKMS(ctx context.Context, arn string) error {
	if arn == "" {
		return fmt.Errorf("missing AWS KMS key ARN")
	}

	region := strings.Split(arn, ":")[3]

	cmd := exec.CommandContext(ctx,
		"aws-encryption-provider",
		"--health-port=127.0.0.1:0",
		"--region="+region,
		"--key="+arn,
		"--listen=/tmp/aws-cred-socket.sock")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		err := cmd.Wait()
		select {
		case <-ctx.Done():
			// ignore error if we are shutting down
		default:
			log.Fatalf("aws-encryption-provider exited: %v", err)
		}
	}()

	return nil
}

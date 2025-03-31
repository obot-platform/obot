package encryption

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/obot-platform/obot/logger"
	"k8s.io/apiserver/pkg/server/options/encryptionconfig"
)

var log = logger.Package()

type Options struct {
	AWSKMSKeyARN         string `usage:"The ARN of the AWS KMS key to use for encrypting credential storage. Only used with the AWS encryption provider." env:"OBOT_AWS_KMS_KEY_ARN" name:"aws-kms-key-arn"`
	GCPKMSKeyURI         string `usage:"The URI of the Google Cloud KMS key to use for encrypting credential storage. Only used with the GCP encryption provider." env:"OBOT_GCP_KMS_KEY_URI" name:"gcp-kms-key-uri"`
	AzureKeyVaultName    string `usage:"The name of the Azure Key Vault to use for encrypting credential storage. Only used with the Azure encryption provider." env:"OBOT_AZURE_KEY_VAULT_NAME" name:"azure-key-vault-name"`
	AzureKeyName         string `usage:"The name of the Azure Key Vault key to use for encrypting credential storage. Only used with the Azure encryption provider." env:"OBOT_AZURE_KEY_NAME" name:"azure-key-vault-key-name"`
	AzureKeyVersion      string `usage:"The version of the Azure Key Vault key to use for encrypting credential storage. Only used with the Azure encryption provider." env:"OBOT_AZURE_KEY_VERSION" name:"azure-key-vault-key-version"`
	EncryptionProvider   string `usage:"The encryption provider to use. Options are AWS, GCP, None, or Custom. Default is None." default:"None"`
	EncryptionConfigFile string `usage:"The path to the encryption configuration file. Only used with the Custom encryption provider."`
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
		if o.EncryptionConfigFile == "" {
			return fmt.Errorf("missing custom encryption config file")
		}
	case "none", "":
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
		if err := setUpAWSKMS(ctx, opts.AWSKMSKeyARN, opts.EncryptionConfigFile); err != nil {
			return nil, fmt.Errorf("failed to setup AWS KMS: %w", err)
		}
	case "gcp":
		if err := setUpGoogleKMS(ctx, opts.GCPKMSKeyURI, opts.EncryptionConfigFile); err != nil {
			return nil, fmt.Errorf("failed to setup Google Cloud KMS: %w", err)
		}
	case "azure":
		if err := setUpAzureKeyVault(ctx, opts.AzureKeyVaultName, opts.AzureKeyName, opts.AzureKeyVersion, opts.EncryptionConfigFile); err != nil {
			return nil, fmt.Errorf("failed to setup Azure Key Vault: %w", err)
		}
	}

	if opts.EncryptionConfigFile != "" {
		log.Infof("Encryption: Using encryption config file: %s", opts.EncryptionConfigFile)
		return encryptionconfig.LoadEncryptionConfig(ctx, opts.EncryptionConfigFile, false, "obot")
	}

	log.Warnf("Encryption: No encryption config file provided, using unencrypted storage")
	return nil, nil
}

func setUpAzureKeyVault(ctx context.Context, keyvaultName, keyName, keyVersion, configFile string) error {
	if keyvaultName == "" || keyName == "" || keyVersion == "" {
		return fmt.Errorf("missing Azure Key Vault configuration")
	}

	if err := os.Setenv("GPTSCRIPT_ENCRYPTION_CONFIG_FILE", configFile); err != nil {
		return fmt.Errorf("failed to set GPTSCRIPT_ENCRYPTION_CONFIG_FILE: %w", err)
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

func setUpGoogleKMS(ctx context.Context, kmsKeyURI, configFile string) error {
	if kmsKeyURI == "" {
		return fmt.Errorf("missing GCP KMS key URI")
	}

	if err := os.Setenv("GPTSCRIPT_ENCRYPTION_CONFIG_FILE", configFile); err != nil {
		return fmt.Errorf("failed to set GPTSCRIPT_ENCRYPTION_CONFIG_FILE: %w", err)
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

func setUpAWSKMS(ctx context.Context, arn, configFile string) error {
	if arn == "" {
		return fmt.Errorf("missing AWS KMS key ARN")
	}

	if err := os.Setenv("GPTSCRIPT_ENCRYPTION_CONFIG_FILE", configFile); err != nil {
		return fmt.Errorf("failed to set GPTSCRIPT_ENCRYPTION_CONFIG_FILE: %w", err)
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

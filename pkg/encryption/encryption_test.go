package encryption

import (
	"context"
	"testing"
)

func TestOptionsValidate(t *testing.T) {
	tests := []struct {
		name        string
		opts        Options
		expectError bool
		errorMsg    string
	}{
		{
			name: "AWS provider with valid ARN",
			opts: Options{
				EncryptionProvider: "aws",
				AWSKMSKeyARN:       "arn:aws:kms:us-west-2:123456789012:key/12345678-1234-1234-1234-123456789012",
			},
			expectError: false,
		},
		{
			name: "AWS provider with missing ARN",
			opts: Options{
				EncryptionProvider: "aws",
				AWSKMSKeyARN:       "",
			},
			expectError: true,
			errorMsg:    "missing AWS KMS key ARN",
		},
		{
			name: "AWS provider case insensitive",
			opts: Options{
				EncryptionProvider: "AWS",
				AWSKMSKeyARN:       "arn:aws:kms:us-west-2:123456789012:key/test",
			},
			expectError: false,
		},
		{
			name: "GCP provider with valid key URI",
			opts: Options{
				EncryptionProvider: "gcp",
				GCPKMSKeyURI:       "projects/my-project/locations/global/keyRings/my-ring/cryptoKeys/my-key",
			},
			expectError: false,
		},
		{
			name: "GCP provider with missing key URI",
			opts: Options{
				EncryptionProvider: "gcp",
				GCPKMSKeyURI:       "",
			},
			expectError: true,
			errorMsg:    "missing GCP KMS key URI",
		},
		{
			name: "GCP provider case insensitive",
			opts: Options{
				EncryptionProvider: "GCP",
				GCPKMSKeyURI:       "projects/my-project/locations/global/keyRings/my-ring/cryptoKeys/my-key",
			},
			expectError: false,
		},
		{
			name: "Azure provider with all required fields",
			opts: Options{
				EncryptionProvider: "azure",
				AzureKeyVaultName:  "my-vault",
				AzureKeyName:       "my-key",
				AzureKeyVersion:    "v1",
			},
			expectError: false,
		},
		{
			name: "Azure provider missing vault name",
			opts: Options{
				EncryptionProvider: "azure",
				AzureKeyVaultName:  "",
				AzureKeyName:       "my-key",
				AzureKeyVersion:    "v1",
			},
			expectError: true,
			errorMsg:    "missing Azure Key Vault configuration",
		},
		{
			name: "Azure provider missing key name",
			opts: Options{
				EncryptionProvider: "azure",
				AzureKeyVaultName:  "my-vault",
				AzureKeyName:       "",
				AzureKeyVersion:    "v1",
			},
			expectError: true,
			errorMsg:    "missing Azure Key Vault configuration",
		},
		{
			name: "Azure provider missing key version",
			opts: Options{
				EncryptionProvider: "azure",
				AzureKeyVaultName:  "my-vault",
				AzureKeyName:       "my-key",
				AzureKeyVersion:    "",
			},
			expectError: true,
			errorMsg:    "missing Azure Key Vault configuration",
		},
		{
			name: "Azure provider case insensitive",
			opts: Options{
				EncryptionProvider: "AZURE",
				AzureKeyVaultName:  "my-vault",
				AzureKeyName:       "my-key",
				AzureKeyVersion:    "v1",
			},
			expectError: false,
		},
		{
			name: "Custom provider with config file",
			opts: Options{
				EncryptionProvider:   "custom",
				EncryptionConfigFile: "/path/to/config.yaml",
			},
			expectError: false,
		},
		{
			name: "Custom provider without config file",
			opts: Options{
				EncryptionProvider:   "custom",
				EncryptionConfigFile: "",
			},
			expectError: true,
			errorMsg:    "missing custom encryption config file",
		},
		{
			name: "Custom provider case insensitive",
			opts: Options{
				EncryptionProvider:   "CUSTOM",
				EncryptionConfigFile: "/path/to/config.yaml",
			},
			expectError: false,
		},
		{
			name: "None provider with no config file",
			opts: Options{
				EncryptionProvider:   "none",
				EncryptionConfigFile: "",
			},
			expectError: false,
		},
		{
			name: "None provider with config file (invalid)",
			opts: Options{
				EncryptionProvider:   "none",
				EncryptionConfigFile: "/path/to/config.yaml",
			},
			expectError: true,
			errorMsg:    "encryption config file provided but encryption provider is set to 'none', use 'custom' encryption provider instead",
		},
		{
			name: "Empty provider with no config file",
			opts: Options{
				EncryptionProvider:   "",
				EncryptionConfigFile: "",
			},
			expectError: false,
		},
		{
			name: "Empty provider with config file (invalid)",
			opts: Options{
				EncryptionProvider:   "",
				EncryptionConfigFile: "/path/to/config.yaml",
			},
			expectError: true,
			errorMsg:    "encryption config file provided but encryption provider is set to 'none', use 'custom' encryption provider instead",
		},
		{
			name: "Invalid provider",
			opts: Options{
				EncryptionProvider: "invalid",
			},
			expectError: true,
			errorMsg:    "invalid encryption provider invalid",
		},
		{
			name: "Unsupported provider name",
			opts: Options{
				EncryptionProvider: "vault",
			},
			expectError: true,
			errorMsg:    "invalid encryption provider vault",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error containing %q but got nil", tt.errorMsg)
					return
				}
				if tt.errorMsg != "" && err.Error() != tt.errorMsg {
					t.Errorf("expected error %q but got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestOptionsValidate_SetsConfigFile(t *testing.T) {
	tests := []struct {
		name           string
		opts           Options
		expectedConfig string
	}{
		{
			name: "AWS sets config file",
			opts: Options{
				EncryptionProvider: "aws",
				AWSKMSKeyARN:       "arn:aws:kms:us-west-2:123456789012:key/test",
			},
			expectedConfig: "/aws-encryption.yaml",
		},
		{
			name: "GCP sets config file",
			opts: Options{
				EncryptionProvider: "gcp",
				GCPKMSKeyURI:       "projects/my-project/locations/global/keyRings/my-ring/cryptoKeys/my-key",
			},
			expectedConfig: "/gcp-encryption.yaml",
		},
		{
			name: "Azure sets config file",
			opts: Options{
				EncryptionProvider: "azure",
				AzureKeyVaultName:  "my-vault",
				AzureKeyName:       "my-key",
				AzureKeyVersion:    "v1",
			},
			expectedConfig: "/azure-encryption.yaml",
		},
		{
			name: "Custom keeps provided config file",
			opts: Options{
				EncryptionProvider:   "custom",
				EncryptionConfigFile: "/custom/path.yaml",
			},
			expectedConfig: "/custom/path.yaml",
		},
		{
			name: "None keeps empty config file",
			opts: Options{
				EncryptionProvider:   "none",
				EncryptionConfigFile: "",
			},
			expectedConfig: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.opts.EncryptionConfigFile != tt.expectedConfig {
				t.Errorf("expected config file %q but got %q", tt.expectedConfig, tt.opts.EncryptionConfigFile)
			}
		})
	}
}

func TestInit_ValidatesOptions(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		opts        Options
		expectError bool
		errorMsg    string
	}{
		{
			name: "Invalid AWS config returns error",
			opts: Options{
				EncryptionProvider: "aws",
				AWSKMSKeyARN:       "", // Missing required field
			},
			expectError: true,
			errorMsg:    "missing AWS KMS key ARN",
		},
		{
			name: "Invalid GCP config returns error",
			opts: Options{
				EncryptionProvider: "gcp",
				GCPKMSKeyURI:       "", // Missing required field
			},
			expectError: true,
			errorMsg:    "missing GCP KMS key URI",
		},
		{
			name: "Invalid Azure config returns error",
			opts: Options{
				EncryptionProvider: "azure",
				AzureKeyVaultName:  "my-vault",
				AzureKeyName:       "", // Missing required field
				AzureKeyVersion:    "v1",
			},
			expectError: true,
			errorMsg:    "missing Azure Key Vault configuration",
		},
		{
			name: "Invalid custom config returns error",
			opts: Options{
				EncryptionProvider:   "custom",
				EncryptionConfigFile: "", // Missing required field
			},
			expectError: true,
			errorMsg:    "missing custom encryption config file",
		},
		{
			name: "Invalid provider returns error",
			opts: Options{
				EncryptionProvider: "invalid-provider",
			},
			expectError: true,
			errorMsg:    "invalid encryption provider invalid-provider",
		},
		{
			name: "None provider returns no error",
			opts: Options{
				EncryptionProvider: "none",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := Init(ctx, tt.opts)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error containing %q but got nil", tt.errorMsg)
					return
				}
				if tt.errorMsg != "" && err.Error() != tt.errorMsg {
					t.Errorf("expected error %q but got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestInit_NoneProvider(t *testing.T) {
	ctx := context.Background()

	opts := Options{
		EncryptionProvider: "none",
	}

	config, configFile, err := Init(ctx, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config != nil {
		t.Errorf("expected nil config but got %v", config)
	}

	if configFile != "" {
		t.Errorf("expected empty config file but got %q", configFile)
	}
}

func TestInit_EmptyProvider(t *testing.T) {
	ctx := context.Background()

	opts := Options{
		EncryptionProvider: "",
	}

	config, configFile, err := Init(ctx, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config != nil {
		t.Errorf("expected nil config but got %v", config)
	}

	if configFile != "" {
		t.Errorf("expected empty config file but got %q", configFile)
	}
}

func TestSetUpAWSKMS_MissingARN(t *testing.T) {
	ctx := context.Background()

	err := setUpAWSKMS(ctx, "", "/aws-encryption.yaml")
	if err == nil {
		t.Fatal("expected error but got nil")
	}

	expectedMsg := "missing AWS KMS key ARN"
	if err.Error() != expectedMsg {
		t.Errorf("expected error %q but got %q", expectedMsg, err.Error())
	}
}

func TestSetUpGoogleKMS_MissingURI(t *testing.T) {
	ctx := context.Background()

	err := setUpGoogleKMS(ctx, "", "/gcp-encryption.yaml")
	if err == nil {
		t.Fatal("expected error but got nil")
	}

	expectedMsg := "missing GCP KMS key URI"
	if err.Error() != expectedMsg {
		t.Errorf("expected error %q but got %q", expectedMsg, err.Error())
	}
}

func TestSetUpAzureKeyVault_MissingConfig(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name           string
		keyvaultName   string
		keyName        string
		keyVersion     string
		expectedErrMsg string
	}{
		{
			name:           "missing all fields",
			keyvaultName:   "",
			keyName:        "",
			keyVersion:     "",
			expectedErrMsg: "missing Azure Key Vault configuration",
		},
		{
			name:           "missing keyvault name",
			keyvaultName:   "",
			keyName:        "my-key",
			keyVersion:     "v1",
			expectedErrMsg: "missing Azure Key Vault configuration",
		},
		{
			name:           "missing key name",
			keyvaultName:   "my-vault",
			keyName:        "",
			keyVersion:     "v1",
			expectedErrMsg: "missing Azure Key Vault configuration",
		},
		{
			name:           "missing key version",
			keyvaultName:   "my-vault",
			keyName:        "my-key",
			keyVersion:     "",
			expectedErrMsg: "missing Azure Key Vault configuration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := setUpAzureKeyVault(ctx, tt.keyvaultName, tt.keyName, tt.keyVersion, "/azure-encryption.yaml")
			if err == nil {
				t.Fatal("expected error but got nil")
			}

			if err.Error() != tt.expectedErrMsg {
				t.Errorf("expected error %q but got %q", tt.expectedErrMsg, err.Error())
			}
		})
	}
}

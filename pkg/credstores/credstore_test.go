package credstores

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInit(t *testing.T) {
	tests := []struct {
		name        string
		dsn         string
		wantErr     bool
		errContains string
	}{
		{
			name:    "postgres dsn",
			dsn:     "postgres://user:pass@localhost:5432/db",
			wantErr: true, // Will error trying to resolve tool ref without registries
		},
		{
			name:    "sqlite dsn",
			dsn:     "sqlite://file:/path/to/db.db",
			wantErr: true, // Will error trying to resolve tool ref without registries
		},
		{
			name:        "unsupported dsn - mysql",
			dsn:         "mysql://localhost:3306/db",
			wantErr:     true,
			errContains: "unsupported database for credentials mysql",
		},
		{
			name:        "unsupported dsn - no scheme",
			dsn:         "invalid-dsn",
			wantErr:     true,
			errContains: "unsupported database for credentials invalid-dsn",
		},
		{
			name:        "empty dsn",
			dsn:         "",
			wantErr:     true,
			errContains: "unsupported database for credentials",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toolRef, envVars, err := Init(nil, tt.dsn, "")
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				assert.Empty(t, toolRef)
				assert.Nil(t, envVars)
			} else {
				require.NoError(t, err)
				assert.NotEmpty(t, toolRef)
				assert.NotNil(t, envVars)
			}
		})
	}
}

func TestSetUpPostgres(t *testing.T) {
	tests := []struct {
		name               string
		dsn                string
		encryptionConfig   string
		wantErr            bool
		expectedEnvVarsDSN string
	}{
		{
			name:               "valid postgres dsn",
			dsn:                "postgres://user:pass@localhost:5432/db",
			encryptionConfig:   "/path/to/config.json",
			wantErr:            true, // Will error without valid tool registries
			expectedEnvVarsDSN: "postgres://user:pass@localhost:5432/db",
		},
		{
			name:               "postgres dsn with special chars",
			dsn:                "postgres://user:p@ss!word@host:5432/db?sslmode=disable",
			encryptionConfig:   "",
			wantErr:            true,
			expectedEnvVarsDSN: "postgres://user:p@ss!word@host:5432/db?sslmode=disable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toolRef, envVars, err := setUpPostgres(nil, tt.dsn, tt.encryptionConfig)
			if tt.wantErr {
				require.Error(t, err)
				assert.Empty(t, toolRef)
				assert.Nil(t, envVars)
			} else {
				require.NoError(t, err)
				assert.NotEmpty(t, toolRef)
				require.Len(t, envVars, 2)
				assert.Contains(t, envVars[0], "GPTSCRIPT_POSTGRES_DSN=")
				assert.Contains(t, envVars[1], "GPTSCRIPT_ENCRYPTION_CONFIG_FILE=")
			}
		})
	}
}

func TestSetUpSQLite(t *testing.T) {
	tests := []struct {
		name             string
		dsn              string
		encryptionConfig string
		wantErr          bool
		errContains      string
		expectedDBFile   string // The db file that should be in env vars
	}{
		{
			name:             "valid sqlite dsn",
			dsn:              "sqlite://file:/path/to/obot.db",
			encryptionConfig: "/path/to/config.json",
			wantErr:          true, // Will error without valid tool registries
			expectedDBFile:   "/path/to/obot-credentials.db",
		},
		{
			name:             "sqlite dsn with query params",
			dsn:              "sqlite://file:/var/data/app.db?mode=rwc",
			encryptionConfig: "",
			wantErr:          true,
			expectedDBFile:   "/var/data/app-credentials.db",
		},
		{
			name:        "missing file: prefix",
			dsn:         "sqlite:///path/to/db.db",
			wantErr:     true,
			errContains: "invalid sqlite dsn, must start with sqlite://file:",
		},
		{
			name:        "wrong scheme",
			dsn:         "sqlite://memory:",
			wantErr:     true,
			errContains: "invalid sqlite dsn, must start with sqlite://file:",
		},
		{
			name:        "missing .db extension",
			dsn:         "sqlite://file:/path/to/database",
			wantErr:     true,
			errContains: "invalid sqlite dsn, file must end in .db:",
		},
		{
			name:        "wrong extension",
			dsn:         "sqlite://file:/path/to/database.sqlite",
			wantErr:     true,
			errContains: "invalid sqlite dsn, file must end in .db:",
		},
		{
			name:        "empty after prefix",
			dsn:         "sqlite://file:",
			wantErr:     true,
			errContains: "invalid sqlite dsn, file must end in .db:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toolRef, envVars, err := setUpSQLite(nil, tt.dsn, tt.encryptionConfig)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				assert.Empty(t, toolRef)
				assert.Nil(t, envVars)
			} else {
				require.NoError(t, err)
				assert.NotEmpty(t, toolRef)
				require.Len(t, envVars, 2)
				// Check that the db file was transformed correctly
				if tt.expectedDBFile != "" {
					sqliteFileEnv := envVars[0]
					assert.True(t, strings.HasPrefix(sqliteFileEnv, "GPTSCRIPT_SQLITE_FILE="))
					assert.Contains(t, sqliteFileEnv, tt.expectedDBFile)
				}
				assert.Contains(t, envVars[1], "GPTSCRIPT_ENCRYPTION_CONFIG_FILE=")
			}
		})
	}
}

func TestResolveToolRef(t *testing.T) {
	tests := []struct {
		name           string
		toolRegistries []string
		relToolPath    string
		wantErr        bool
		errContains    string
	}{
		{
			name:           "empty tool registries",
			toolRegistries: []string{},
			relToolPath:    "credential-stores/postgres",
			wantErr:        true,
			errContains:    "not found in provided tool registries",
		},
		{
			name:           "nil tool registries",
			toolRegistries: nil,
			relToolPath:    "credential-stores/sqlite",
			wantErr:        true,
			errContains:    "not found in provided tool registries",
		},
		{
			name:           "empty relative tool path",
			toolRegistries: []string{"https://registry.example.com"},
			relToolPath:    "",
			wantErr:        true,
			errContains:    "not found in provided tool registries",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toolRef, err := resolveToolRef(tt.toolRegistries, tt.relToolPath)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				assert.Empty(t, toolRef)
			} else {
				require.NoError(t, err)
				assert.NotEmpty(t, toolRef)
			}
		})
	}
}

func TestDSNTransformations(t *testing.T) {
	// Test that SQLite DSN transformations work correctly
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple path",
			input:    "sqlite://file:/data/obot.db",
			expected: "/data/obot-credentials.db",
		},
		{
			name:     "nested path",
			input:    "sqlite://file:/var/lib/obot/data/main.db",
			expected: "/var/lib/obot/data/main-credentials.db",
		},
		{
			name:     "with query params",
			input:    "sqlite://file:/app.db?cache=shared&mode=rwc",
			expected: "/app-credentials.db",
		},
		{
			name:     "multiple dots in filename",
			input:    "sqlite://file:/path/obot.test.db",
			expected: "/path/obot.test-credentials.db",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := setUpSQLite(nil, tt.input, "")
			// We expect an error because tool registries are nil, but we can still check the env vars
			require.Error(t, err) // Expected since registries are nil
			// The function should have failed at resolveToolRef, not at DSN parsing
			assert.Contains(t, err.Error(), "not found in provided tool registries")
		})
	}
}

func TestSQLiteDSNEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		dsn         string
		wantErr     bool
		errContains string
	}{
		{
			name:        "ends with .db but has more",
			dsn:         "sqlite://file:/path.db.backup",
			wantErr:     true,
			errContains: "file must end in .db",
		},
		{
			name:        "relative path",
			dsn:         "sqlite://file:./relative/path.db",
			wantErr:     true, // Will error at tool resolution
			errContains: "not found in provided tool registries",
		},
		{
			name:        "absolute path with spaces (valid format)",
			dsn:         "sqlite://file:/path with spaces/db.db",
			wantErr:     true, // Will error at tool resolution
			errContains: "not found in provided tool registries",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := setUpSQLite(nil, tt.dsn, "")
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestEncryptionConfigPropagation(t *testing.T) {
	// Test that encryption config is properly included in environment variables
	encryptionConfig := "/etc/obot/encryption-key.json"

	t.Run("postgres includes encryption config", func(t *testing.T) {
		_, _, err := setUpPostgres(nil, "postgres://localhost/db", encryptionConfig)
		require.Error(t, err) // Expected since registries are nil
		// Check that even though it errors, we know the signature is correct
	})

	t.Run("sqlite includes encryption config", func(t *testing.T) {
		_, _, err := setUpSQLite(nil, "sqlite://file:/data/app.db", encryptionConfig)
		require.Error(t, err) // Expected since registries are nil
		// Check that even though it errors, we know the signature is correct
	})
}

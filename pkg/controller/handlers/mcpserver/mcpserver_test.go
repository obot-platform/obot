package mcpserver

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
)

func TestConfigurationHasDrifted(t *testing.T) {
	tests := []struct {
		name           string
		serverManifest types.MCPServerManifest
		entryManifest  types.MCPServerCatalogEntryManifest
		expectedDrift  bool
		expectedError  bool
	}{
		{
			name: "no drift - identical manifests",
			serverManifest: types.MCPServerManifest{
				Env:     []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "KEY1", Name: "key1"}}},
				Command: "test-command",
				Args:    []string{"arg1", "arg2"},
			},
			entryManifest: types.MCPServerCatalogEntryManifest{
				Env:     []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "KEY1", Name: "key1"}}},
				Command: "test-command",
				Args:    []string{"arg1", "arg2"},
			},
			expectedDrift: false,
			expectedError: false,
		},
		{
			name: "no drift - empty manifests",
			serverManifest: types.MCPServerManifest{
				Env:     []types.MCPEnv{},
				Command: "",
				Args:    []string{},
			},
			entryManifest: types.MCPServerCatalogEntryManifest{
				Env:     []types.MCPEnv{},
				Command: "",
				Args:    []string{},
			},
			expectedDrift: false,
			expectedError: false,
		},
		{
			name: "no drift - different env order (order doesn't matter)",
			serverManifest: types.MCPServerManifest{
				Env:     []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "KEY1", Name: "key1"}}, {MCPHeader: types.MCPHeader{Key: "KEY2", Name: "key2"}}},
				Command: "test-command",
				Args:    []string{"arg1", "arg2"},
			},
			entryManifest: types.MCPServerCatalogEntryManifest{
				Env:     []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "KEY2", Name: "key2"}}, {MCPHeader: types.MCPHeader{Key: "KEY1", Name: "key1"}}},
				Command: "test-command",
				Args:    []string{"arg1", "arg2"},
			},
			expectedDrift: false,
			expectedError: false,
		},
		{
			name: "drift - different env values",
			serverManifest: types.MCPServerManifest{
				Env:     []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "KEY1", Name: "key1"}}},
				Command: "test-command",
				Args:    []string{"arg1", "arg2"},
			},
			entryManifest: types.MCPServerCatalogEntryManifest{
				Env:     []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "KEY2", Name: "key2"}}},
				Command: "test-command",
				Args:    []string{"arg1", "arg2"},
			},
			expectedDrift: true,
			expectedError: false,
		},
		{
			name: "drift - different command",
			serverManifest: types.MCPServerManifest{
				Env:     []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "KEY1", Name: "key1"}}},
				Command: "test-command",
				Args:    []string{"arg1", "arg2"},
			},
			entryManifest: types.MCPServerCatalogEntryManifest{
				Env:     []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "KEY1", Name: "key1"}}},
				Command: "different-command",
				Args:    []string{"arg1", "arg2"},
			},
			expectedDrift: true,
			expectedError: false,
		},
		{
			name: "drift - different args (order matters)",
			serverManifest: types.MCPServerManifest{
				Env:     []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "KEY1", Name: "key1"}}},
				Command: "test-command",
				Args:    []string{"arg1", "arg2"},
			},
			entryManifest: types.MCPServerCatalogEntryManifest{
				Env:     []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "KEY1", Name: "key1"}}},
				Command: "test-command",
				Args:    []string{"arg2", "arg1"},
			},
			expectedDrift: true,
			expectedError: false,
		},
		{
			name: "no drift - different headers order (order doesn't matter)",
			serverManifest: types.MCPServerManifest{
				Env:     []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "KEY1", Name: "key1"}}},
				Headers: []types.MCPHeader{{Key: "HEADER1", Name: "header1"}, {Key: "HEADER2", Name: "header2"}},
				URL:     "http://example.com",
			},
			entryManifest: types.MCPServerCatalogEntryManifest{
				Env:      []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "KEY1", Name: "key1"}}},
				Headers:  []types.MCPHeader{{Key: "HEADER2", Name: "header2"}, {Key: "HEADER1", Name: "header1"}},
				FixedURL: "http://example.com",
			},
			expectedDrift: false,
			expectedError: false,
		},
		{
			name: "drift - different header values",
			serverManifest: types.MCPServerManifest{
				Env:     []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "KEY1", Name: "key1"}}},
				Headers: []types.MCPHeader{{Key: "HEADER1", Name: "header1"}},
				URL:     "http://example.com",
			},
			entryManifest: types.MCPServerCatalogEntryManifest{
				Env:      []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "KEY1", Name: "key1"}}},
				Headers:  []types.MCPHeader{{Key: "HEADER2", Name: "header2"}},
				FixedURL: "http://example.com",
			},
			expectedDrift: true,
			expectedError: false,
		},
		{
			name: "drift - different fixed URL",
			serverManifest: types.MCPServerManifest{
				Env:     []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "KEY1", Name: "key1"}}},
				Headers: []types.MCPHeader{{Key: "HEADER1", Name: "header1"}},
				URL:     "http://example.com",
			},
			entryManifest: types.MCPServerCatalogEntryManifest{
				Env:      []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "KEY1", Name: "key1"}}},
				Headers:  []types.MCPHeader{{Key: "HEADER1", Name: "header1"}},
				FixedURL: "http://different.com",
			},
			expectedDrift: true,
			expectedError: false,
		},
		{
			name: "no drift - matching fixed URL",
			serverManifest: types.MCPServerManifest{
				Env:     []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "KEY1", Name: "key1"}}},
				Headers: []types.MCPHeader{{Key: "HEADER1", Name: "header1"}},
				URL:     "http://example.com",
			},
			entryManifest: types.MCPServerCatalogEntryManifest{
				Env:      []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "KEY1", Name: "key1"}}},
				Headers:  []types.MCPHeader{{Key: "HEADER1", Name: "header1"}},
				FixedURL: "http://example.com",
			},
			expectedDrift: false,
			expectedError: false,
		},
		{
			name: "drift - different hostname",
			serverManifest: types.MCPServerManifest{
				Env:     []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "KEY1", Name: "key1"}}},
				Headers: []types.MCPHeader{{Key: "HEADER1", Name: "header1"}},
				URL:     "http://example.com:8080/path",
			},
			entryManifest: types.MCPServerCatalogEntryManifest{
				Env:      []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "KEY1", Name: "key1"}}},
				Headers:  []types.MCPHeader{{Key: "HEADER1", Name: "header1"}},
				Hostname: "different.example.com",
			},
			expectedDrift: true,
			expectedError: false,
		},
		{
			name: "no drift - matching hostname",
			serverManifest: types.MCPServerManifest{
				Env:     []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "KEY1", Name: "key1"}}},
				Headers: []types.MCPHeader{{Key: "HEADER1", Name: "header1"}},
				URL:     "http://example.com:8080/path",
			},
			entryManifest: types.MCPServerCatalogEntryManifest{
				Env:      []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "KEY1", Name: "key1"}}},
				Headers:  []types.MCPHeader{{Key: "HEADER1", Name: "header1"}},
				Hostname: "example.com",
			},
			expectedDrift: false,
			expectedError: false,
		},
		{
			name: "error - invalid URL when hostname is specified",
			serverManifest: types.MCPServerManifest{
				Env:     []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "KEY1", Name: "key1"}}},
				Headers: []types.MCPHeader{{Key: "HEADER1", Name: "header1"}},
				URL:     "://invalid-url",
			},
			entryManifest: types.MCPServerCatalogEntryManifest{
				Env:      []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "KEY1", Name: "key1"}}},
				Headers:  []types.MCPHeader{{Key: "HEADER1", Name: "header1"}},
				Hostname: "example.com",
			},
			expectedDrift: true,
			expectedError: true,
		},
		{
			name: "no drift - nil slices treated as empty",
			serverManifest: types.MCPServerManifest{
				Env:     nil,
				Command: "test-command",
				Args:    nil,
				Headers: nil,
			},
			entryManifest: types.MCPServerCatalogEntryManifest{
				Env:     []types.MCPEnv{},
				Command: "test-command",
				Args:    []string{},
				Headers: []types.MCPHeader{},
			},
			expectedDrift: false,
			expectedError: false,
		},
		{
			name: "drift - different slice lengths",
			serverManifest: types.MCPServerManifest{
				Env:     []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "KEY1", Name: "key1"}}},
				Command: "test-command",
				Args:    []string{"arg1", "arg2"},
			},
			entryManifest: types.MCPServerCatalogEntryManifest{
				Env:     []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "KEY1", Name: "key1"}}, {MCPHeader: types.MCPHeader{Key: "KEY2", Name: "key2"}}},
				Command: "test-command",
				Args:    []string{"arg1", "arg2"},
			},
			expectedDrift: true,
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			drifted, err := configurationHasDrifted(tt.serverManifest, tt.entryManifest)

			if tt.expectedError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}

			if drifted != tt.expectedDrift {
				t.Errorf("Expected drift=%v, got drift=%v", tt.expectedDrift, drifted)
			}
		})
	}
}

func TestConfigurationHasDrifted_EdgeCases(t *testing.T) {
	t.Run("hostname check with URL without hostname", func(t *testing.T) {
		serverManifest := types.MCPServerManifest{
			URL: "file:///path/to/file",
		}
		entryManifest := types.MCPServerCatalogEntryManifest{
			Hostname: "example.com",
		}

		drifted, err := configurationHasDrifted(serverManifest, entryManifest)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if !drifted {
			t.Errorf("Expected drift=true for file URL vs hostname, got drift=false")
		}
	})

	t.Run("both fixedURL and hostname specified - fixedURL takes precedence", func(t *testing.T) {
		serverManifest := types.MCPServerManifest{
			URL: "http://example.com",
		}
		entryManifest := types.MCPServerCatalogEntryManifest{
			FixedURL: "http://different.com",
			Hostname: "example.com", // This should be ignored since FixedURL is set
		}

		drifted, err := configurationHasDrifted(serverManifest, entryManifest)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if !drifted {
			t.Errorf("Expected drift=true due to different FixedURL, got drift=false")
		}
	})
}

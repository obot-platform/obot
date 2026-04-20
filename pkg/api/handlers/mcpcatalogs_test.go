package handlers

import (
	"testing"

	mcpcataloghandler "github.com/obot-platform/obot/pkg/controller/handlers/mcpcatalog"
)

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "basic spaces",
			input:    "My App Config",
			expected: "my-app-config",
		},
		{
			name:     "single quotes and spaces",
			input:    "My App's Config",
			expected: "my-app-s-config",
		},
		{
			name:     "special characters",
			input:    "Test_Server@1.0!",
			expected: "test-server-1-0",
		},
		{
			name:     "mixed case with symbols",
			input:    "Special!@#$%Characters",
			expected: "special-characters",
		},
		{
			name:     "multiple consecutive spaces",
			input:    "App   With   Spaces",
			expected: "app-with-spaces",
		},
		{
			name:     "leading and trailing spaces",
			input:    "  App Config  ",
			expected: "app-config",
		},
		{
			name:     "leading and trailing special chars",
			input:    "!!!App Config***",
			expected: "app-config",
		},
		{
			name:     "only special characters",
			input:    "!@#$%^&*()",
			expected: "",
		},
		{
			name:     "already valid name",
			input:    "my-valid-name",
			expected: "my-valid-name",
		},
		{
			name:     "numbers and hyphens",
			input:    "app-v1.2.3",
			expected: "app-v1-2-3",
		},
		{
			name:     "unicode characters",
			input:    "café-résumé",
			expected: "caf-r-sum",
		},
		{
			name:     "long name gets truncated",
			input:    "this-is-a-very-long-name-that-exceeds-the-kubernetes-limit-of-sixty-three-characters-and-should-be-truncated",
			expected: "this-is-a-very-long-name-that-exceeds-the-kubernetes-limit-of-s",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only spaces",
			input:    "   ",
			expected: "",
		},
		{
			name:     "uppercase letters",
			input:    "UPPERCASE-NAME",
			expected: "uppercase-name",
		},
		{
			name:     "mixed alphanumeric with symbols",
			input:    "App123@#$Test456",
			expected: "app123-test456",
		},
		{
			name:     "parentheses and brackets",
			input:    "App (v2.0) [Production]",
			expected: "app-v2-0-production",
		},
		{
			name:     "dots and underscores",
			input:    "my.app_name.config",
			expected: "my-app-name-config",
		},
		{
			name:     "consecutive special chars become single dash",
			input:    "app!!!@@@###config",
			expected: "app-config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeMCPCatalogEntryName(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeName(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNormalizeNameKubernetesCompliance(t *testing.T) {
	testCases := []string{
		"My App's Config",
		"Test_Server@1.0!",
		"Special!@#$%Characters",
		"App   With   Spaces",
		"  App Config  ",
		"café-résumé",
		"UPPERCASE-NAME",
		"App (v2.0) [Production]",
	}

	for _, input := range testCases {
		t.Run(input, func(t *testing.T) {
			result := normalizeMCPCatalogEntryName(input)

			// Test length constraint
			if len(result) > 63 {
				t.Errorf("NormalizeName(%q) = %q has length %d, exceeds 63 characters", input, result, len(result))
			}

			// Test character constraints (only lowercase alphanumeric and hyphens)
			for i, r := range result {
				if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
					t.Errorf("NormalizeName(%q) = %q contains invalid character %q at position %d", input, result, r, i)
				}
			}

			// Test that it doesn't start or end with hyphen (unless empty)
			if len(result) > 0 {
				if result[0] == '-' {
					t.Errorf("NormalizeName(%q) = %q starts with hyphen", input, result)
				}
				if result[len(result)-1] == '-' {
					t.Errorf("NormalizeName(%q) = %q ends with hyphen", input, result)
				}
			}
		})
	}
}

func TestFindCredentialTransfer(t *testing.T) {
	toolName := func(u string) string { return mcpcataloghandler.CatalogCredentialToolName(u) }

	urlA := "https://example.com/org/repoA.git"
	urlB := "https://example.com/org/repoB.git"
	urlC := "https://example.com/org/repoC.git"

	tests := []struct {
		name           string
		oldURLs        []string
		newURLs        []string
		existingCreds  map[string]struct{}
		newURLCreds    map[string]string
		wantOld        string
		wantNew        string
		shouldTransfer bool
	}{
		{
			name:           "simple rename transfers credential",
			oldURLs:        []string{urlA},
			newURLs:        []string{urlB},
			existingCreds:  map[string]struct{}{toolName(urlA): {}},
			newURLCreds:    map[string]string{urlB: "*"},
			wantOld:        urlA,
			wantNew:        urlB,
			shouldTransfer: true,
		},
		{
			name:           "new URL already has a credential - no transfer",
			oldURLs:        []string{urlA},
			newURLs:        []string{urlB},
			existingCreds:  map[string]struct{}{toolName(urlA): {}, toolName(urlB): {}},
			newURLCreds:    map[string]string{urlB: "*"},
			shouldTransfer: false,
		},
		{
			name:           "no credential for old URL - no transfer",
			oldURLs:        []string{urlA},
			newURLs:        []string{urlB},
			existingCreds:  map[string]struct{}{},
			newURLCreds:    map[string]string{urlB: "*"},
			shouldTransfer: false,
		},
		{
			name:           "new URL does not signal transfer (*) - no transfer",
			oldURLs:        []string{urlA},
			newURLs:        []string{urlB},
			existingCreds:  map[string]struct{}{toolName(urlA): {}},
			newURLCreds:    map[string]string{urlB: "actual-token"},
			shouldTransfer: false,
		},
		{
			name:           "multiple removed URLs with creds - ambiguous, no transfer",
			oldURLs:        []string{urlA, urlC},
			newURLs:        []string{urlB},
			existingCreds:  map[string]struct{}{toolName(urlA): {}, toolName(urlC): {}},
			newURLCreds:    map[string]string{urlB: "*"},
			shouldTransfer: false,
		},
		{
			name:           "multiple new URLs wanting transfer - ambiguous, no transfer",
			oldURLs:        []string{urlA},
			newURLs:        []string{urlB, urlC},
			existingCreds:  map[string]struct{}{toolName(urlA): {}},
			newURLCreds:    map[string]string{urlB: "*", urlC: "*"},
			shouldTransfer: false,
		},
		{
			name:           "URL unchanged - no transfer",
			oldURLs:        []string{urlA},
			newURLs:        []string{urlA},
			existingCreds:  map[string]struct{}{toolName(urlA): {}},
			newURLCreds:    map[string]string{},
			shouldTransfer: false,
		},
		{
			name:           "empty lists - no transfer",
			oldURLs:        []string{},
			newURLs:        []string{},
			existingCreds:  map[string]struct{}{},
			newURLCreds:    map[string]string{},
			shouldTransfer: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOld, gotNew, shouldTransfer := findCredentialTransfer(tt.oldURLs, tt.newURLs, tt.existingCreds, tt.newURLCreds)
			if shouldTransfer != tt.shouldTransfer {
				t.Errorf("findCredentialTransfer() shouldTransfer = %v, want %v", shouldTransfer, tt.shouldTransfer)
			}
			if shouldTransfer {
				if gotOld != tt.wantOld {
					t.Errorf("findCredentialTransfer() oldURL = %q, want %q", gotOld, tt.wantOld)
				}
				if gotNew != tt.wantNew {
					t.Errorf("findCredentialTransfer() newURL = %q, want %q", gotNew, tt.wantNew)
				}
			}
		})
	}
}

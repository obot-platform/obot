package auth

import (
	"testing"
)

func TestValidateGroupIDPrefix(t *testing.T) {
	tests := []struct {
		name      string
		prefix    string
		wantError bool
	}{
		{
			name:   "provider without groups",
			prefix: "",
		},
		{
			name:   "valid prefix",
			prefix: "entra/",
		},
		{
			name:   "valid mixed case and numeric prefix",
			prefix: "Entra2/",
		},
		{
			name:      "missing slash",
			prefix:    "entra",
			wantError: true,
		},
		{
			name:      "empty namespace",
			prefix:    "/",
			wantError: true,
		},
		{
			name:      "nested namespace",
			prefix:    "entra/custom/",
			wantError: true,
		},
		{
			name:      "hyphen",
			prefix:    "azure-ad/",
			wantError: true,
		},
		{
			name:      "underscore",
			prefix:    "azure_ad/",
			wantError: true,
		},
		{
			name:      "punctuation",
			prefix:    "entra%/",
			wantError: true,
		},
		{
			name:      "whitespace",
			prefix:    "entra id/",
			wantError: true,
		},
		{
			name:      "extra trailing slash",
			prefix:    "entra//",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGroupIDPrefix(tt.prefix)
			if tt.wantError && err == nil {
				t.Fatal("expected an error")
			}
			if !tt.wantError && err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestValidateGroupID(t *testing.T) {
	tests := []struct {
		name      string
		groupID   string
		prefix    string
		wantError bool
	}{
		{
			name:    "matching group",
			groupID: "entra/engineering",
			prefix:  "entra/",
		},
		{
			name:      "different provider",
			groupID:   "okta/engineering",
			prefix:    "entra/",
			wantError: true,
		},
		{
			name:      "prefix alone",
			groupID:   "entra/",
			prefix:    "entra/",
			wantError: true,
		},
		{
			name:      "missing declared prefix",
			groupID:   "entra/engineering",
			prefix:    "",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGroupID(tt.groupID, tt.prefix)
			if tt.wantError && err == nil {
				t.Fatal("expected an error")
			}
			if !tt.wantError && err != nil {
				t.Fatal(err)
			}
		})
	}
}

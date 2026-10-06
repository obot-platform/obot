package adapter

import (
	"testing"
)

func TestOktaNativeGroupID(t *testing.T) {
	tests := []struct {
		name    string
		prefix  string
		groupID string
		want    string
	}{
		{
			name:    "native group ID",
			prefix:  "okta/",
			groupID: "okta/00g1a2b3c4d5e6f7g8h9",
			want:    "00g1a2b3c4d5e6f7g8h9",
		},
		{
			name:    "group created through SCIM",
			prefix:  "okta/",
			groupID: "okta/7a0d2c1e-9f3b-4e5a-8c6d-2b1f0e9d8c7a",
		},
		{
			name:    "legacy name-based ID",
			prefix:  "okta/",
			groupID: "okta/Engineering",
		},
		{
			name:    "user ID",
			prefix:  "okta/",
			groupID: "okta/00u1a2b3c4d5e6f7g8h9",
		},
		{
			name:    "another provider's prefix",
			prefix:  "okta/",
			groupID: "entra/00g1a2b3c4d5e6f7g8h9",
		},
		{
			name:    "non-alphanumeric characters",
			prefix:  "okta/",
			groupID: "okta/00g1a2b3c4d5e6f7g8-9",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (okta{}).NativeGroupID(tt.prefix, tt.groupID); got != tt.want {
				t.Fatalf("NativeGroupID(%q, %q) = %q, want %q", tt.prefix, tt.groupID, got, tt.want)
			}
		})
	}
}

func TestOktaGroupConsoleURL(t *testing.T) {
	const groupID = "00g1a2b3c4d5e6f7g8h9"

	tests := []struct {
		name    string
		issuer  string
		groupID string
		want    string
	}{
		{
			name:    "org authorization server",
			issuer:  "https://dev-123456.okta.com",
			groupID: groupID,
			want:    "https://dev-123456-admin.okta.com/admin/group/00g1a2b3c4d5e6f7g8h9",
		},
		{
			name:    "preview org",
			issuer:  "https://acme.oktapreview.com/oauth2/default",
			groupID: groupID,
			want:    "https://acme-admin.oktapreview.com/admin/group/00g1a2b3c4d5e6f7g8h9",
		},
		{
			name:    "custom domain",
			issuer:  "https://login.acme.com/oauth2/default",
			groupID: groupID,
		},
		{
			name:    "no native group ID",
			issuer:  "https://acme.okta.com",
			groupID: "",
		},
		{
			name:    "not HTTPS",
			issuer:  "http://acme.okta.com",
			groupID: groupID,
		},
		{
			name:    "nested subdomain",
			issuer:  "https://evil.acme.okta.com",
			groupID: groupID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (okta{}).GroupConsoleURL(tt.issuer, tt.groupID); got != tt.want {
				t.Fatalf("GroupConsoleURL(%q, %q) = %q, want %q", tt.issuer, tt.groupID, got, tt.want)
			}
		})
	}
}

package client

import (
	"testing"

	"github.com/obot-platform/obot/pkg/gateway/types"
)

func TestPingFederateProfilePreservesIdentity(t *testing.T) {
	u := &types.User{Username: "JDOE", Email: "JDOE", HashedEmail: "hash-email", HashedUsername: "hash-user"}
	id := &types.Identity{ProviderUserID: "stable-subject"}
	applyPingFederateProfile(u, id, map[string]any{"name": "Jane Doe", "email": "someone-else", "email_display_domain": "example.com"})
	if u.Username != "JDOE" || u.Email != "JDOE" || u.HashedEmail != "hash-email" || u.HashedUsername != "hash-user" || id.ProviderUserID != "stable-subject" {
		t.Fatal("profile changed identity")
	}
	if u.DisplayName != "Jane Doe" || u.EmailDisplayDomain != "example.com" {
		t.Fatalf("profile = %+v", u)
	}
	apiUser := types.ConvertUser(u, false, "pingfederate-auth-provider")
	if apiUser.Email != "JDOE" || apiUser.EmailDisplayDomain != "example.com" {
		t.Fatalf("API profile = %+v", apiUser)
	}
	applyPingFederateProfile(u, id, map[string]any{"name": "Jane Doe", "email_display_domain": ""})
	if u.EmailDisplayDomain != "" {
		t.Fatal("display domain did not clear")
	}
}

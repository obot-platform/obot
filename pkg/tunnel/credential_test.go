package tunnel

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestCredentialLifecycle(t *testing.T) {
	token, propertyValue, err := NewCredential()
	if err != nil {
		t.Fatal(err)
	}
	rawToken, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("token is not base64url: %v", err)
	}
	if len(rawToken) != 64 {
		t.Fatalf("raw token length = %d, want 64", len(rawToken))
	}
	if propertyValue == token {
		t.Fatal("property contains the recoverable token")
	}
	if !CredentialMatches(propertyValue, token) {
		t.Fatal("stored credential does not match its token")
	}
	if CredentialMatches(propertyValue, token+"wrong") {
		t.Fatal("stored credential matched a different token")
	}
	credentialID, err := CredentialID(propertyValue)
	if err != nil {
		t.Fatal(err)
	}
	if len(credentialID) != 32 {
		t.Fatalf("credential ID length = %d, want 32", len(credentialID))
	}

	preview, err := CredentialPreview(propertyValue)
	if err != nil {
		t.Fatal(err)
	}
	encodedPreview, ok := strings.CutSuffix(preview, "...")
	if !ok {
		t.Fatalf("preview = %q, want display ellipsis", preview)
	}
	rawPreview, err := base64.RawURLEncoding.DecodeString(encodedPreview)
	if err != nil {
		t.Fatalf("preview is not base64url: %v", err)
	}
	if len(rawPreview) != 16 {
		t.Fatalf("raw preview length = %d, want 16", len(rawPreview))
	}
}

func TestCredentialRejectsMalformedProperty(t *testing.T) {
	if CredentialMatches("not-json", "token") {
		t.Fatal("malformed credential matched")
	}
	if _, err := CredentialID("not-json"); err == nil {
		t.Fatal("malformed credential returned an ID")
	}
	if _, err := CredentialPreview("not-json"); err == nil {
		t.Fatal("malformed credential returned a preview")
	}
}

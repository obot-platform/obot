package localauth

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestTokenSignerRoundTrip(t *testing.T) {
	signer, err := newTokenSigner()
	if err != nil {
		t.Fatal(err)
	}

	const email = "user@example.com"
	token := signer.sign(email)

	got, err := signer.verify(token)
	if err != nil {
		t.Fatal(err)
	}
	if got != email {
		t.Fatalf("expected %q, got %q", email, got)
	}
}

func TestTokenSignerRejectsInvalidTokens(t *testing.T) {
	signer, err := newTokenSigner()
	if err != nil {
		t.Fatal(err)
	}
	otherSigner, err := newTokenSigner()
	if err != nil {
		t.Fatal(err)
	}

	validToken := signer.sign("user@example.com")
	payload, signature, ok := strings.Cut(validToken, ".")
	if !ok {
		t.Fatal("expected signed token to contain a separator")
	}

	tests := map[string]string{
		"missing signature":   payload,
		"malformed signature": payload + ".%%%",
		"forged payload":      base64.RawURLEncoding.EncodeToString([]byte("attacker@example.com")) + "." + signature,
		"different signer":    otherSigner.sign("user@example.com"),
		"malformed payload":   "%%%." + base64.RawURLEncoding.EncodeToString(signer.mac("%%%")),
	}

	for name, token := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := signer.verify(token); err == nil {
				t.Fatal("expected token verification to fail")
			}
		})
	}
}

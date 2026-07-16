package localauth

import (
	"errors"
	"strings"
	"testing"
)

func TestHashPasswordRoundTrip(t *testing.T) {
	const password = "correct horse battery staple"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	if strings.Contains(hash, password) {
		t.Fatal("hash contains the plaintext password")
	}

	if err := VerifyPassword(hash, password); err != nil {
		t.Fatalf("failed to verify correct password: %v", err)
	}

	if err := VerifyPassword(hash, password+"!"); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("expected ErrInvalidPassword for the wrong password, got %v", err)
	}
}

func TestHashPasswordIsSalted(t *testing.T) {
	const password = "correct horse battery staple"

	first, err := HashPassword(password)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	second, err := HashPassword(password)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	if first == second {
		t.Fatal("hashing the same password twice produced the same hash, so it is not salted")
	}
}

func TestHashPasswordRejectsShortPasswords(t *testing.T) {
	if _, err := HashPassword(strings.Repeat("a", MinPasswordLength-1)); err == nil {
		t.Fatal("expected an error for a password below the minimum length")
	}
}

func TestVerifyPasswordRejectsMalformedHashes(t *testing.T) {
	for _, hash := range []string{
		"",
		"plaintext",
		"$argon2i$v=19$m=19456,t=2,p=1$c2FsdHNhbHRzYWx0c2E$aGFzaA",
		"$argon2id$v=19$m=19456,t=2,p=1$not-base64!$aGFzaA",
	} {
		if err := VerifyPassword(hash, "correct horse battery staple"); err == nil {
			t.Fatalf("expected an error for malformed hash %q", hash)
		}
	}
}

func TestEmailDomainAllowed(t *testing.T) {
	tests := []struct {
		domains, email string
		want           bool
	}{
		{"*", "user@example.com", true},
		{"example.com", "user@example.com", true},
		{"example.com", "user@EXAMPLE.com", false}, // emails are normalized before this check
		{"example.com, other.com", "user@other.com", true},
		{"@example.com", "user@example.com", true},
		{"example.com", "user@notexample.com", false},
		{"example.com", "user@evil.com", false},
		{"", "user@example.com", false},
		{"*", "not-an-email", false},
	}

	for _, tt := range tests {
		if got := emailDomainAllowed(tt.domains, tt.email); got != tt.want {
			t.Errorf("emailDomainAllowed(%q, %q) = %v, want %v", tt.domains, tt.email, got, tt.want)
		}
	}
}

func TestRedirectTarget(t *testing.T) {
	tests := []struct {
		rd, want string
	}{
		{"", "/"},
		{"/admin", "/admin"},
		{"//evil.com", "/"},
		{"https://evil.com", "/"},
		{"javascript:alert(1)", "/"},
	}

	for _, tt := range tests {
		if got := redirectTarget(tt.rd); got != tt.want {
			t.Errorf("redirectTarget(%q) = %q, want %q", tt.rd, got, tt.want)
		}
	}
}

package auth

import (
	"testing"
)

func TestSafeRedirectPath(t *testing.T) {
	tests := []struct {
		rd   string
		want string
	}{
		{
			rd:   "",
			want: "/",
		},
		{
			rd:   "/admin",
			want: "/admin",
		},
		{
			rd:   "/admin?rd=1#f",
			want: "/admin?rd=1#f",
		},

		{
			rd:   "//evil.com",
			want: "/",
		},
		{
			rd:   "https://evil.com",
			want: "/",
		},
		{
			rd:   "javascript:alert(1)",
			want: "/",
		},

		// Browsers resolve a backslash like a slash, so these are protocol-relative in disguise.
		{
			rd:   `/\evil.com`,
			want: "/",
		},
		{
			rd:   `/\/evil.com`,
			want: "/",
		},
		{
			rd:   `\\evil.com`,
			want: "/",
		},

		// A percent-encoded backslash is not a separator, so this stays on our origin.
		{
			rd:   "/%5Cevil.com",
			want: "/%5Cevil.com",
		},
	}

	for _, tt := range tests {
		if got := SafeRedirectPath(tt.rd); got != tt.want {
			t.Errorf("SafeRedirectPath(%q) = %q, want %q", tt.rd, got, tt.want)
		}
	}
}

package accesstoken

import (
	"context"
	"testing"
)

func TestContextWithAccessToken(t *testing.T) {
	tests := []struct {
		name        string
		accessToken string
	}{
		{
			name:        "valid access token",
			accessToken: "token123",
		},
		{
			name:        "empty access token",
			accessToken: "",
		},
		{
			name:        "long access token",
			accessToken: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ",
		},
		{
			name:        "access token with special characters",
			accessToken: "token-with_special.chars!@#$",
		},
		{
			name:        "access token with spaces",
			accessToken: "token with spaces",
		},
		{
			name:        "unicode access token",
			accessToken: "令牌123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			ctx = ContextWithAccessToken(ctx, tt.accessToken)

			got := GetAccessToken(ctx)
			if got != tt.accessToken {
				t.Errorf("GetAccessToken() = %q, want %q", got, tt.accessToken)
			}
		})
	}
}

func TestGetAccessToken(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		want string
	}{
		{
			name: "context without access token",
			ctx:  context.Background(),
			want: "",
		},
		{
			name: "context with access token",
			ctx:  ContextWithAccessToken(context.Background(), "my-token"),
			want: "my-token",
		},
		{
			name: "context with empty access token",
			ctx:  ContextWithAccessToken(context.Background(), ""),
			want: "",
		},
		{
			name: "context with JWT token",
			ctx:  ContextWithAccessToken(context.Background(), "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"),
			want: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
		},
		{
			name: "nested context values preserved",
			ctx: ContextWithAccessToken(
				context.WithValue(context.Background(), "other-key", "other-value"),
				"token123",
			),
			want: "token123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetAccessToken(tt.ctx)
			if got != tt.want {
				t.Errorf("GetAccessToken() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetAccessToken_NilContext(t *testing.T) {
	// This test verifies behavior with nil context
	// GetAccessToken should handle nil gracefully or panic (we test for panic)
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil context, but didn't panic")
		}
	}()

	GetAccessToken(nil)
}

func TestContextOverwrite(t *testing.T) {
	// Test that setting a new access token overwrites the old one
	ctx := context.Background()
	ctx = ContextWithAccessToken(ctx, "first-token")
	ctx = ContextWithAccessToken(ctx, "second-token")

	got := GetAccessToken(ctx)
	if got != "second-token" {
		t.Errorf("GetAccessToken() = %q, want %q (token should be overwritten)", got, "second-token")
	}
}

func TestMultipleContexts(t *testing.T) {
	// Test that multiple contexts can have different access tokens
	ctx1 := ContextWithAccessToken(context.Background(), "token1")
	ctx2 := ContextWithAccessToken(context.Background(), "token2")

	got1 := GetAccessToken(ctx1)
	got2 := GetAccessToken(ctx2)

	if got1 != "token1" {
		t.Errorf("GetAccessToken(ctx1) = %q, want %q", got1, "token1")
	}
	if got2 != "token2" {
		t.Errorf("GetAccessToken(ctx2) = %q, want %q", got2, "token2")
	}
}

func TestContextIsolation(t *testing.T) {
	// Test that setting access token creates a new context that doesn't affect the parent
	parentCtx := context.Background()
	childCtx := ContextWithAccessToken(parentCtx, "child-token")

	parentToken := GetAccessToken(parentCtx)
	childToken := GetAccessToken(childCtx)

	if parentToken != "" {
		t.Errorf("parent context should not have access token, got %q", parentToken)
	}
	if childToken != "child-token" {
		t.Errorf("child context GetAccessToken() = %q, want %q", childToken, "child-token")
	}
}

func TestRoundTrip(t *testing.T) {
	// Test round-trip: set token, get token, verify it matches
	tokens := []string{
		"simple",
		"",
		"with-dashes-and_underscores",
		"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0",
		"!@#$%^&*()",
	}

	for _, token := range tokens {
		t.Run(token, func(t *testing.T) {
			ctx := ContextWithAccessToken(context.Background(), token)
			got := GetAccessToken(ctx)
			if got != token {
				t.Errorf("round-trip failed: got %q, want %q", got, token)
			}
		})
	}
}

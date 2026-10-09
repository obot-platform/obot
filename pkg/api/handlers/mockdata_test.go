package handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/obot-platform/obot/pkg/gateway/client"
)

type mockDataUserLimitProviderFunc func(context.Context) (client.UserLimit, error)

func (f mockDataUserLimitProviderFunc) UserLimit(ctx context.Context) (client.UserLimit, error) {
	return f(ctx)
}

func TestResolveMockDataUserLimit(t *testing.T) {
	providerError := errors.New("license unavailable")
	tests := []struct {
		name          string
		limit         client.UserLimit
		providerError error
		wantError     string
	}{
		{
			name: "limited",
			limit: client.UserLimit{
				Maximum: 20,
			},
		},
		{
			name: "unlimited",
			limit: client.UserLimit{
				Unlimited: true,
			},
		},
		{
			name:      "invalid limit",
			wantError: "invalid user limit 0",
		},
		{
			name:          "provider failure",
			providerError: providerError,
			wantError:     "failed to resolve user limit: license unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveMockDataUserLimit(t.Context(), mockDataUserLimitProviderFunc(func(context.Context) (client.UserLimit, error) {
				return tt.limit, tt.providerError
			}))
			if tt.wantError != "" {
				if err == nil || err.Error() != tt.wantError {
					t.Fatalf("resolveMockDataUserLimit() error = %v, want %q", err, tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.limit {
				t.Fatalf("resolveMockDataUserLimit() = %+v, want %+v", got, tt.limit)
			}
		})
	}
}

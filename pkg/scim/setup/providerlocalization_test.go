package setup

import (
	"context"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/i18n"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func TestProviderDisplayName(t *testing.T) {
	tests := []struct {
		name    string
		locale  string
		display string
		want    string
	}{
		{name: "translated", locale: "ja", display: "${local_provider_name}", want: "ローカル"},
		{name: "background English", display: "${local_provider_name}", want: "Local"},
		{name: "literal", locale: "ja", display: "Custom provider", want: "Custom provider"},
		{name: "resource fallback", locale: "ja", want: "example-auth-provider"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.locale != "" {
				ctx = i18n.WithLocale(ctx, tt.locale)
			}
			provider := v1.AuthProvider{
				Name: "example-auth-provider",
				Spec: v1.AuthProviderSpec{AuthProviderManifest: types.AuthProviderManifest{
					CommonProviderMetadata: types.CommonProviderMetadata{Name: tt.display},
				}},
			}
			if got := displayName(ctx, provider); got != tt.want {
				t.Fatalf("displayName() = %q, want %q", got, tt.want)
			}
		})
	}
}

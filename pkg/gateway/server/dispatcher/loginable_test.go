package dispatcher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/auth"
	"github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	activeProviderName = "active-auth-provider"
	stagedProviderName = "staged-auth-provider"
)

// A staged provider is reachable only by the browser holding the verification it was issued for.
// Without that binding, opening a verification would turn the not-yet-activated replacement into a
// login path for anyone who can name it.
func TestLoginableAuthProviderRequiresTheVerificationCookie(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	services, err := sservices.New(sservices.Config{DSN: "sqlite://:memory:"})
	if err != nil {
		t.Fatalf("failed to create storage services: %v", err)
	}
	db, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	if err != nil {
		t.Fatalf("failed to create gateway db: %v", err)
	}
	if err := db.AutoMigrate(); err != nil {
		t.Fatalf("failed to auto-migrate: %v", err)
	}

	provider := func(name string) *v1.AuthProvider {
		return &v1.AuthProvider{
			Namespace: system.DefaultNamespace,
			Name:      name,
			Spec: v1.AuthProviderSpec{
				AuthProviderManifest: apitypes.AuthProviderManifest{
					CommonProviderMetadata: apitypes.CommonProviderMetadata{
						Name:                            name,
						RequiredConfigurationParameters: []apitypes.ProviderConfigurationParameter{{Name: "TOKEN"}},
					},
				},
			},
			Status: v1.AuthProviderStatus{Configured: name == activeProviderName},
		}
	}

	storageClient := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(provider(activeProviderName), provider(stagedProviderName)).
		WithIndex(&v1.AuthProvider{}, "status.configured", func(o kclient.Object) []string {
			return []string{o.(*v1.AuthProvider).Get("status.configured")}
		}).
		Build()

	gatewayClient := client.New(ctx, db, storageClient, nil, nil, nil, nil, time.Hour, 1, 90, 90, 90, true)
	t.Cleanup(func() { _ = gatewayClient.Close() })

	// The active provider is configured; the replacement only exists in the staged context.
	if err := gatewayClient.UpsertCredential(ctx, gatewaytypes.Credential{
		Context: activeProviderName, Name: activeProviderName, Secrets: map[string]string{"TOKEN": "active"},
	}); err != nil {
		t.Fatalf("configuring the active provider: %v", err)
	}
	if err := gatewayClient.UpsertCredential(ctx, gatewaytypes.Credential{
		Context: system.ReplacementAuthProviderCredentialContext, Name: stagedProviderName, Secrets: map[string]string{"TOKEN": "staged"},
	}); err != nil {
		t.Fatalf("staging the replacement provider: %v", err)
	}

	const verifyID = "live-verification"
	if err := gatewayClient.CreateTokenRequest(ctx, &gatewaytypes.TokenRequest{
		ID:               verifyID,
		Purpose:          gatewaytypes.TokenRequestPurposeAuthProviderVerify,
		RequestExpiresAt: time.Now().Add(auth.AuthProviderVerifyWindow),
	}); err != nil {
		t.Fatalf("opening a verification: %v", err)
	}

	d := &Dispatcher{client: storageClient, gatewayClient: gatewayClient}

	tests := []struct {
		name     string
		provider string
		cookie   string
		want     bool
	}{
		{
			name:     "active provider needs no cookie",
			provider: activeProviderName,
			want:     true,
		},
		// The whole point of the fix: an open verification is not a global switch.
		{
			name:     "staged provider without a cookie",
			provider: stagedProviderName,
		},
		{
			name:     "staged provider with an unknown verification",
			provider: stagedProviderName,
			cookie:   "not-the-verification",
		},
		{
			name:     "staged provider with the live verification",
			provider: stagedProviderName,
			cookie:   verifyID,
			want:     true,
		},
		{
			name:     "unrelated provider is never loginable",
			provider: "some-other-provider",
			cookie:   verifyID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/oauth2/start", nil)
			if tt.cookie != "" {
				req.AddCookie(&http.Cookie{Name: auth.AuthProviderVerifyCookie, Value: tt.cookie})
			}

			got, err := d.LoginableAuthProvider(ctx, req, tt.provider)
			if err != nil {
				t.Fatalf("LoginableAuthProvider: %v", err)
			}
			if got != tt.want {
				t.Errorf("LoginableAuthProvider(%q) = %v, want %v", tt.provider, got, tt.want)
			}
		})
	}
}

package mcpgateway

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/user"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// deactivateThroughSCIM has the identity provider of provider deactivate the user with the native ID nativeID and the
// email email through SCIM, as it does in production, setting up the provider's SCIM connection first if it has none.
// A user who signed in with that ID is bound to their account, and disabled.
func deactivateThroughSCIM(t *testing.T, c *gatewayclient.Client, provider gatewayclient.AuthProviderRef, nativeID, email string) error {
	t.Helper()

	conn, err := c.SCIMConnectionForAuthProvider(t.Context(), provider.Namespace, provider.Name)
	if err != nil {
		return err
	}
	if conn == nil {
		if conn, _, err = c.CreateSCIMConnection(t.Context(), gatewayclient.CreateSCIMConnectionOptions{
			AuthProviderNamespace: provider.Namespace,
			AuthProviderName:      provider.Name,
			GroupIDPrefix:         "okta/",
			Origin:                gatewaytypes.SCIMConnectionOriginSCIMFirst,
		}); err != nil {
			return err
		}
	}
	_, err = c.CreateSCIMUser(t.Context(), conn, gatewayclient.SCIMUserInput{
		UserName:   email,
		ExternalID: nativeID,
		Active:     new(false),
		Profile: gatewaytypes.SCIMUserProfile{
			Emails: []gatewaytypes.SCIMMultiValue{
				{
					Value:   email,
					Primary: true,
				},
			},
		},
	}, gatewayclient.SCIMUserCreateOptions{
		UserLimit: gatewayclient.UserLimit{
			Unlimited: true,
		},
		DefaultRole: types.RoleBasic,
	})
	return err
}

func TestCompositeLoopbackTokenRecordsAHostedAgentsOwner(t *testing.T) {
	now := time.Now()

	for _, tt := range []struct {
		name      string
		caller    user.Info
		wantOwner string
	}{
		{
			name: "person",
			caller: &user.DefaultInfo{
				Name: "alice",
				UID:  "7",
				Extra: map[string][]string{
					"email": {"alice@example.com"},
				},
			},
		},
		{
			name: "hosted agent",
			caller: &user.DefaultInfo{
				Name: "hosted-agent:hai1abc",
				UID:  "hosted-agent:hai1abc",
				Extra: map[string][]string{
					principal.HostedAgentOwnerExtra: {"7"},
				},
			},
			wantOwner: "7",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := compositeLoopbackTokenContext(tt.caller, "https://obot.example.com/mcp-connect-composite/vmcp1", "vmcp1", []string{"component"}, now)

			if got.UserID != tt.caller.GetUID() {
				t.Errorf("token user = %q, want the caller %q", got.UserID, tt.caller.GetUID())
			}
			if got.HostedAgentOwnerID != tt.wantOwner {
				t.Errorf("token hosted agent owner = %q, want %q", got.HostedAgentOwnerID, tt.wantOwner)
			}
		})
	}
}

func TestImpersonatorCannotReachTheAgentOfAnInactiveOwner(t *testing.T) {
	services, err := sservices.New(sservices.Config{
		DSN: "sqlite://:memory:",
	})
	if err != nil {
		t.Fatalf("failed to create storage services: %v", err)
	}
	db, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	if err != nil {
		t.Fatalf("failed to create gateway db: %v", err)
	}
	if err := db.AutoMigrate(); err != nil {
		t.Fatalf("failed to migrate gateway db: %v", err)
	}
	storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(&v1.UserDefaultRoleSetting{
		Namespace: system.DefaultNamespace,
		Name:      system.DefaultRoleSettingName,
		Spec: v1.UserDefaultRoleSettingSpec{
			Role: types.RoleBasic,
		},
	}).Build()
	gatewayClient := gatewayclient.New(t.Context(), db, storage, nil, nil, nil, nil, time.Hour, 10, 90, 90, 90, true)
	t.Cleanup(func() { _ = gatewayClient.Close() })

	provider := gatewayclient.AuthProviderRef{
		Namespace: system.DefaultNamespace,
		Name:      "okta-auth-provider",
	}
	owner, err := gatewayClient.EnsureIdentityWithRole(t.Context(), &gatewaytypes.Identity{
		AuthProviderName:      provider.Name,
		AuthProviderNamespace: provider.Namespace,
		ProviderUsername:      "owner",
		ProviderUserID:        "00u-owner",
		Email:                 "owner@example.com",
	}, "", types.RoleBasic, gatewayclient.UserLimit{Unlimited: true})
	if err != nil {
		t.Fatalf("failed to create agent owner: %v", err)
	}

	h := &Handler{}
	req := api.Context{
		Request:       httptest.NewRequest(http.MethodGet, "/mcp-connect/agent", nil),
		GatewayClient: gatewayClient,
	}

	if err := h.checkAgentOwnerAccess(req, fmt.Sprint(owner.ID)); err != nil {
		t.Fatalf("check for an active owner = %v, want allowed", err)
	}

	if err := deactivateThroughSCIM(t, gatewayClient, provider, "00u-owner", owner.Email); err != nil {
		t.Fatalf("failed to disable agent owner: %v", err)
	}
	for name, ownerID := range map[string]string{
		"disabled owner": fmt.Sprint(owner.ID),
		"missing owner":  "4242",
		"invalid owner":  "not-a-user",
	} {
		err := h.checkAgentOwnerAccess(req, ownerID)
		var errHTTP *types.ErrHTTP
		if !errors.As(err, &errHTTP) || errHTTP.Code != http.StatusForbidden {
			t.Errorf("%s: check = %v, want forbidden", name, err)
		}
	}
}

package authn

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestNoAuthPrincipalRecordsTheUserKind(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
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
	storageClient := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(&v1.UserDefaultRoleSetting{
			Namespace: system.DefaultNamespace,
			Name:      system.DefaultRoleSettingName,
			Spec: v1.UserDefaultRoleSettingSpec{
				Role: types2.RoleBasic,
			},
		}).
		Build()
	c := client.New(ctx, db, storageClient, nil, nil, time.Hour, 1, 90, 90, 90, true)
	t.Cleanup(func() {
		cancel()
		_ = c.Close()
	})

	resp, ok, err := NewNoAuth(c).AuthenticateRequest(httptest.NewRequestWithContext(ctx, http.MethodGet, "/", nil))
	if err != nil || !ok {
		t.Fatalf("authenticate = %v, %v; want the no-auth principal", ok, err)
	}
	got, ok := principal.FromUserInfo(resp.User)
	if !ok || got.Kind != principal.KindUser || got.String() != resp.User.GetUID() {
		t.Fatalf("FromUserInfo() = %#v, %v; want the user with UID %q", got, ok, resp.User.GetUID())
	}
}

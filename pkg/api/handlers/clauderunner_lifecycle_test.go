package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/clauderunner"
	"github.com/obot-platform/obot/pkg/controller/handlers/agentinstance"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gt "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/substrate"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDisabledClaudeUserAdmissionAndReconciliation(t *testing.T) {
	// Admission holds a transaction while reading through another connection.
	services, err := storageservices.New(storageservices.Config{DSN: "sqlite://" + filepath.Join(t.TempDir(), "admission.db")})
	require.NoError(t, err)
	services.DB.SQLDB.SetMaxOpenConns(2)
	database, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate())
	gateway := gclient.New(t.Context(), database, nil, nil, nil, nil, nil, time.Hour, 10, 0, 0, 0, false)
	t.Cleanup(func() { _ = gateway.Close() })
	db := services.DB.DB
	ctx := t.Context()
	require.NoError(t, db.Create(&gt.User{ID: 123, Username: "mapped", HashedUsername: "mapped", Email: "mapped@example.com"}).Error)
	env := &gt.ClaudeEnvironment{
		ID: "environment", OrganizationID: "org_test", EnvironmentID: "ccpool_test",
		Profiles: []gt.ClaudeEnvironmentProfile{{Name: "mapped", Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "123"}}}},
	}
	token := strings.Repeat("a", 32)
	require.NoError(t, gateway.SaveClaudeEnvironment(ctx, env, map[string]string{"token": token}))
	require.NoError(t, gateway.SaveClaudeOrganizationAccounts(ctx, env.OrganizationID, []gt.ClaudeOrganizationAccount{{Account: "user_mapped", UserID: "123"}}))
	_, err = clauderunner.AuthorizedEnvironment(ctx, gateway, env.ID, "user_mapped", "123")
	require.NoError(t, err)
	require.NoError(t, db.Model(&gt.User{}).Where("id = ?", 123).Update("disabled_at", time.Now()).Error)
	_, err = clauderunner.AuthorizedEnvironment(ctx, gateway, env.ID, "user_mapped", "123")
	require.True(t, clauderunner.AccessDenied(err))

	// Substitute only the PostgreSQL lock statement for this SQLite fixture.
	require.NoError(t, db.Callback().Raw().Before("gorm:raw").Register("test_advisory_lock", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "pg_advisory_xact_lock") {
			tx.Statement.SQL.Reset()
			tx.Statement.SQL.WriteString("SELECT 1")
			tx.Statement.Vars = nil
		}
	}))
	inst := &v1.AgentInstance{
		Name: "runner", Namespace: system.DefaultNamespace, Finalizers: []string{substrate.Finalizer},
		Spec: v1.AgentInstanceSpec{UserID: "123", ClaudeAccount: "user_mapped", ClaudeEnvironment: env.ID, ClaudeOrder: "order"},
	}
	storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(inst).Build()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"account":"user_mapped","pool":"ccpool_test","order":"new","session":"session","workOrder":"`+token+`"}`))
	req.Header.Set("X-Obot-Claude-Environment", env.ID)
	req.Header.Set("X-Obot-Orchestrator-Token", token)
	h := &ClaudeRunnerHandler{Substrate: &substrate.Client{}}
	err = h.Spawn(api.Context{Request: req, ResponseWriter: httptest.NewRecorder(), GatewayClient: gateway, Storage: storage})
	require.ErrorContains(t, err, "does not have access")

	controller := &agentinstance.Handler{Gateway: gateway}
	require.NoError(t, controller.Reconcile(router.Request{Ctx: ctx, Client: storage, Object: inst}, nil))
	require.NoError(t, storage.Get(ctx, kclient.ObjectKeyFromObject(inst), inst))
	require.NotNil(t, inst.DeletionTimestamp, "revoked sessions must go through resource cleanup")
}

func TestAdmissionRecordsDefaultImages(t *testing.T) {
	// Admission holds a transaction while reading through another connection.
	services, err := storageservices.New(storageservices.Config{DSN: "sqlite://" + filepath.Join(t.TempDir(), "admission.db")})
	require.NoError(t, err)
	services.DB.SQLDB.SetMaxOpenConns(2)
	database, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate())
	gateway := gclient.New(t.Context(), database, nil, nil, nil, nil, nil, time.Hour, 10, 0, 0, 0, false)
	t.Cleanup(func() { _ = gateway.Close() })
	db := services.DB.DB
	ctx := t.Context()
	require.NoError(t, db.Create(&gt.User{ID: 123, Username: "mapped", HashedUsername: "mapped", Email: "mapped@example.com"}).Error)
	// The environment sets no images, so it runs Obot's defaults.
	env := &gt.ClaudeEnvironment{
		ID: "environment", OrganizationID: "org_test", EnvironmentID: "ccpool_test",
		Profiles: []gt.ClaudeEnvironmentProfile{{Name: "mapped", Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "123"}}}},
		Runtime:  gt.ClaudeRuntime{LLMProxy: true},
	}
	token := strings.Repeat("a", 32)
	require.NoError(t, gateway.SaveClaudeEnvironment(ctx, env, map[string]string{"token": token}))
	require.NoError(t, gateway.SaveClaudeOrganizationAccounts(ctx, env.OrganizationID, []gt.ClaudeOrganizationAccount{{Account: "user_mapped", UserID: "123"}}))
	require.NoError(t, db.Callback().Raw().Before("gorm:raw").Register("test_advisory_lock", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "pg_advisory_xact_lock") {
			tx.Statement.SQL.Reset()
			tx.Statement.SQL.WriteString("SELECT 1")
			tx.Statement.Vars = nil
		}
	}))

	storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build()
	runnerImage := "example/runner@sha256:" + strings.Repeat("b", 64)
	h := &ClaudeRunnerHandler{Substrate: &substrate.Client{Config: substrate.Config{RunnerImage: runnerImage, WorkerImage: "example/worker:default"}}}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"account":"user_mapped","pool":"ccpool_test","order":"order","session":"session","workOrder":"`+token+`"}`))
	req.Header.Set("X-Obot-Claude-Environment", env.ID)
	req.Header.Set("X-Obot-Orchestrator-Token", token)
	require.NoError(t, h.Spawn(api.Context{Request: req, ResponseWriter: httptest.NewRecorder(), GatewayClient: gateway, Storage: storage}))

	var instances v1.AgentInstanceList
	require.NoError(t, storage.List(ctx, &instances))
	require.Len(t, instances.Items, 1)
	var runtime gt.ClaudeRuntime
	require.NoError(t, json.Unmarshal([]byte(instances.Items[0].Spec.ClaudeRuntime), &runtime))
	require.Equal(t, runnerImage, runtime.Image, "the session records the image it runs")
	require.Equal(t, "example/worker:default", runtime.WorkerImage)
	require.True(t, runtime.LLMProxy, "the session records whether its environment uses the LLM proxy")

	stored, err := gateway.ClaudeEnvironment(ctx, env.ID)
	require.NoError(t, err)
	require.Empty(t, stored.Runtime.Image, "admission does not pin the environment's image")
	require.Empty(t, stored.Runtime.WorkerImage)
}

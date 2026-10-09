package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
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
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newClaudeLifecycleTestGateway(t *testing.T, users ...gt.User) *gclient.Client {
	t.Helper()
	services, err := storageservices.New(storageservices.Config{DSN: "sqlite://" + filepath.Join(t.TempDir(), "claude.db")})
	require.NoError(t, err)
	services.DB.SQLDB.SetMaxOpenConns(2)
	database, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate())
	for i := range users {
		require.NoError(t, services.DB.DB.Create(&users[i]).Error)
	}
	gateway := gclient.New(t.Context(), database, nil, nil, nil, nil, nil, time.Hour, 10, 0, 0, 0, false)
	t.Cleanup(func() { _ = gateway.Close() })
	// Substitute the PostgreSQL advisory lock for the SQLite fixture.
	require.NoError(t, services.DB.DB.Callback().Raw().Before("gorm:raw").Register("test_advisory_lock", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "pg_advisory_xact_lock") {
			tx.Statement.SQL.Reset()
			tx.Statement.SQL.WriteString("SELECT 1")
			tx.Statement.Vars = nil
		}
	}))
	return gateway
}

func TestSaveClaudeTagOnlyCreatesUserWhenEnabled(t *testing.T) {
	gateway := newClaudeLifecycleTestGateway(t)
	ctx := t.Context()
	env := &gt.ClaudeEnvironment{ID: "env", EnvironmentID: "ccpool_test"}
	require.NoError(t, gateway.SaveClaudeEnvironment(ctx, env, nil))
	storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build()
	handler := &ClaudeRunnerHandler{Substrate: &substrate.Client{}}
	var originalID string
	for _, enabled := range []bool{false, true, false} {
		request := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(fmt.Sprintf(`{"enabled":%t}`, enabled)))
		request.SetPathValue("environment", env.ID)
		response := httptest.NewRecorder()
		require.NoError(t, handler.SaveClaudeTag(api.Context{
			Request:        request,
			ResponseWriter: response,
			GatewayClient:  gateway,
			Storage:        storage,
			User:           &user.DefaultInfo{Groups: types.RoleAdmin.Groups()},
		}))
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), env))
		require.Equal(t, enabled, env.ClaudeTagEnabled)
		if enabled {
			require.NotEmpty(t, env.ClaudeTagUserID)
			originalID = env.ClaudeTagUserID
		} else {
			require.Equal(t, originalID, env.ClaudeTagUserID)
		}
	}
}

func TestSessionCleanupWithoutSubstrate(t *testing.T) {
	gateway := newClaudeLifecycleTestGateway(t)
	ctx := t.Context()
	env := &gt.ClaudeEnvironment{ID: "env", EnvironmentID: "ccpool_test"}
	require.NoError(t, gateway.SaveClaudeEnvironment(ctx, env, nil))
	require.NoError(t, gateway.EnsureClaudeTagUser(ctx, env))
	owner, err := gateway.UserByID(ctx, env.ClaudeTagUserID)
	require.NoError(t, err)
	inst := &v1.AgentInstance{
		Name:      "runner",
		Namespace: system.DefaultNamespace,
		Spec:      v1.AgentInstanceSpec{ClaudeOrder: "order", UserID: env.ClaudeTagUserID},
	}
	credential, err := gateway.EnsureClaudeCredential(ctx, owner.ID, inst.Name, []string{"vmcp"}, time.Now().Add(time.Hour))
	require.NoError(t, err)
	require.NoError(t, gateway.SaveClaudeWorkOrder(ctx, gt.Credential{
		Context: "claude-runner",
		Name:    inst.Name,
		Secrets: map[string]string{"token": "runner-token"},
	}))
	handler := &agentinstance.Handler{Gateway: gateway}
	require.NoError(t, handler.Cleanup(router.Request{Ctx: ctx, Object: inst}, nil))
	_, err = gateway.ValidateAPIKey(ctx, credential.Secrets["key"])
	require.Error(t, err)
	for _, context := range []string{"substrate-agent", "claude-runner"} {
		_, err := gateway.RevealCredential(ctx, []string{context}, inst.Name)
		require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	}
	require.NoError(t, handler.Cleanup(router.Request{Ctx: ctx, Object: inst}, nil))
}

func TestDeleteEnvironmentRevokesSessionKeysBeforeCleanup(t *testing.T) {
	owner := gt.User{ID: 123, Username: "human", HashedUsername: "human", Email: "human@example.com"}
	gateway := newClaudeLifecycleTestGateway(t, owner)
	ctx := t.Context()

	environment := &gt.ClaudeEnvironment{ID: "env", EnvironmentID: "ccpool_test"}
	require.NoError(t, gateway.SaveClaudeEnvironment(ctx, environment, nil))
	storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build()
	keys := map[string]string{}
	for _, id := range []string{environment.ID, "other-env"} {
		instance := &v1.AgentInstance{
			Name:       id + "-session",
			Namespace:  system.DefaultNamespace,
			Finalizers: []string{substrate.Finalizer},
			Spec: v1.AgentInstanceSpec{
				UserID:            strconv.FormatUint(uint64(owner.ID), 10),
				ClaudeEnvironment: id,
				ClaudeAccount:     "user_human",
			},
		}
		require.NoError(t, storage.Create(ctx, instance))
		credential, err := gateway.EnsureClaudeCredential(ctx, owner.ID, instance.Name, []string{"vmcp"}, time.Now().Add(time.Hour))
		require.NoError(t, err)
		keys[id] = credential.Secrets["key"]
	}

	request := httptest.NewRequest(http.MethodDelete, "/", nil)
	request.SetPathValue("environment", environment.ID)
	response := httptest.NewRecorder()
	handler := &ClaudeRunnerHandler{}
	require.NoError(t, handler.DeleteEnvironment(api.Context{
		Request:        request,
		ResponseWriter: response,
		GatewayClient:  gateway,
		Storage:        storage,
		User:           &user.DefaultInfo{Groups: types.RoleAdmin.Groups()},
	}))
	require.Equal(t, http.StatusNoContent, response.Code)
	_, err := gateway.ClaudeEnvironment(ctx, environment.ID)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	for id, key := range keys {
		var instance v1.AgentInstance
		require.NoError(t, storage.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: id + "-session"}, &instance))
		_, err := gateway.ValidateAPIKey(ctx, key)
		if id == environment.ID {
			require.Error(t, err, "key must be revoked before the finalizer runs")
			require.NotNil(t, instance.DeletionTimestamp)
		} else {
			require.NoError(t, err)
			require.Nil(t, instance.DeletionTimestamp)
		}
	}
}

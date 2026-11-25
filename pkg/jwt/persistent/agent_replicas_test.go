package persistent

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	// replicaTestPostgresDSNEnv names a PostgreSQL URL, whose user can create schemas, to run the replica tests on.
	// Without it they run on a SQLite file that every replica opens.
	replicaTestPostgresDSNEnv = "OBOT_TEST_POSTGRES_DSN"
)

// sharedReplicaDSN returns the DSN of a database for the replicas of one test: an isolated schema of the PostgreSQL
// database that replicaTestPostgresDSNEnv names, or else a new SQLite file.
func sharedReplicaDSN(t *testing.T) string {
	t.Helper()

	postgresDSN := os.Getenv(replicaTestPostgresDSNEnv)
	if postgresDSN == "" {
		return "sqlite://file:" + filepath.Join(t.TempDir(), "obot.db") + "?_journal=WAL&_busy_timeout=30000"
	}

	admin, err := sservices.New(sservices.Config{DSN: postgresDSN})
	require.NoError(t, err)
	schema := "obot_agent_replicas_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	require.NoError(t, admin.DB.DB.Exec("CREATE SCHEMA "+schema).Error)
	t.Cleanup(func() {
		assert.NoError(t, admin.DB.DB.Exec("DROP SCHEMA "+schema+" CASCADE").Error)
		assert.NoError(t, admin.DB.SQLDB.Close())
	})

	u, err := url.Parse(postgresDSN)
	require.NoError(t, err)
	query := u.Query()
	query.Set("search_path", schema)
	u.RawQuery = query.Encode()
	return u.String()
}

// openReplicaGatewayClient opens a gateway client of its own on the database at dsn, as each Obot replica does. The
// first one to open the database migrates it.
func openReplicaGatewayClient(t *testing.T, dsn string, migrate bool) *client.Client {
	t.Helper()

	services, err := sservices.New(sservices.Config{DSN: dsn})
	require.NoError(t, err)
	db, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, migrate)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate())

	storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(&v1.UserDefaultRoleSetting{
		Namespace: system.DefaultNamespace,
		Name:      system.DefaultRoleSettingName,
		Spec: v1.UserDefaultRoleSettingSpec{
			Role: types.RoleBasic,
		},
	}).Build()
	gatewayClient := client.New(t.Context(), db, storage, nil, nil, time.Hour, 10, 90, 90, 90, false)
	t.Cleanup(func() {
		_ = gatewayClient.Close()
		_ = services.DB.SQLDB.Close()
	})
	return gatewayClient
}

// newReplicaTokenServices returns the token services of two replicas, A and B, and a gateway client an administrator's
// request is served with, each with its own connection to one database. A creates the signing key, as the controller
// leader does, and B loads it from the database when it first needs it, as every other replica does.
func newReplicaTokenServices(t *testing.T) (replicaA, replicaB *TokenService, admin *client.Client) {
	t.Helper()

	dsn := sharedReplicaDSN(t)
	gatewayA := openReplicaGatewayClient(t, dsn, true)
	gatewayB := openReplicaGatewayClient(t, dsn, false)
	admin = openReplicaGatewayClient(t, dsn, false)

	var err error
	replicaA, err = NewTokenService(testServerURL, gatewayA)
	require.NoError(t, err)
	require.NoError(t, replicaA.EnsureJWK(t.Context()))
	replicaB, err = NewTokenService(testServerURL, gatewayB)
	require.NoError(t, err)
	return replicaA, replicaB, admin
}

// TestAgentLoopbackTokensAreDeniedOnOtherReplicas checks that a composite loopback token issued to an agent on one
// replica is denied on another from the moment the agent is disabled or deleted there through a third connection, and
// that the other replica issues it no new token meanwhile. Each replica reads the agent with its own connection, so
// nothing but the shared database tells it of the change.
func TestAgentLoopbackTokensAreDeniedOnOtherReplicas(t *testing.T) {
	replicaA, replicaB, admin := newReplicaTokenServices(t)
	agent := createTestAgent(t, admin)
	subject := principal.Agent(agent.ID).String()

	_, token, err := replicaA.NewToken(t.Context(), TokenContext{
		Audience:   testServerURL + "/mcp-connect/server-id",
		ExpiresAt:  NewTime(time.Now().Add(time.Hour)),
		UserID:     subject,
		UserGroups: agentLoopbackGroups,
	})
	require.NoError(t, err)

	authenticates := func(t *testing.T, replica *TokenService) {
		t.Helper()
		_, ok, err := replica.AuthenticateRequest(bearerRequest(token))
		require.NoError(t, err)
		require.True(t, ok)
	}
	denied := func(t *testing.T, replica *TokenService, want types.UserStatus) {
		t.Helper()
		response, ok, err := replica.AuthenticateRequest(bearerRequest(token))
		assert.Nil(t, response)
		assert.False(t, ok)
		deniedErr, isDenied := errors.AsType[*client.AgentAccessDeniedError](err)
		require.True(t, isDenied, "error = %v, want a denial", err)
		assert.Equal(t, agent.ID, deniedErr.AgentID)
		assert.Equal(t, want, deniedErr.Status)

		// The authenticator ahead of a catch-all fails rather than declines it.
		response, ok, err = replica.AgentTokens().AuthenticateRequest(bearerRequest(token))
		assert.Nil(t, response)
		assert.False(t, ok)
		require.Error(t, err)

		// Nor is the agent issued a new token there.
		_, _, err = replica.NewToken(t.Context(), TokenContext{
			Audience:   testServerURL,
			UserID:     subject,
			UserGroups: agentLoopbackGroups,
		})
		deniedErr, isDenied = errors.AsType[*client.AgentAccessDeniedError](err)
		require.True(t, isDenied, "NewToken error = %v, want a denial", err)
		assert.Equal(t, want, deniedErr.Status)
	}

	// B verifies the token that A signed with the key they share through the database, and has read the agent as
	// active before it changes.
	authenticates(t, replicaB)
	authenticates(t, replicaA)

	_, err = admin.DisableAgent(t.Context(), agent.ID, "compromised")
	require.NoError(t, err)
	denied(t, replicaB, types.UserStatusDisabled)
	denied(t, replicaA, types.UserStatusDisabled)

	_, err = admin.EnableAgent(t.Context(), agent.ID)
	require.NoError(t, err)
	authenticates(t, replicaB)

	require.NoError(t, admin.DeleteAgent(t.Context(), agent.ID))
	denied(t, replicaB, types.UserStatusDeleted)
	denied(t, replicaA, types.UserStatusDeleted)
}

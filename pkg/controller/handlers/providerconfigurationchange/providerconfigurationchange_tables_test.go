package providerconfigurationchange

import (
	"fmt"
	"os"
	"testing"
	"time"

	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// providerTablesTest gives the Okta provider of a scimChangeTest a table prefix of its own in the PostgreSQL database
// that OBOT_TEST_POSTGRES_DSN names, and skips the test without one.
type providerTablesTest struct {
	*scimChangeTest
	db     *gorm.DB
	prefix string
}

func newProviderTablesTest(t *testing.T) *providerTablesTest {
	t.Helper()
	dsn := os.Getenv("OBOT_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("OBOT_TEST_POSTGRES_DSN is not set")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)

	s := newSCIMChangeTest(t)
	s.handler.postgresDSN = dsn
	prefix := fmt.Sprintf("tablestest%d_", time.Now().UnixNano())
	var okta v1.AuthProvider
	require.NoError(t, s.client.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: oktaProviderName}, &okta))
	okta.Spec.PostgresTablePrefix = prefix
	require.NoError(t, s.client.Update(t.Context(), &okta))

	p := &providerTablesTest{scimChangeTest: s, db: db, prefix: prefix}
	t.Cleanup(func() {
		for _, table := range p.tables() {
			_ = db.Exec("DROP TABLE IF EXISTS " + table).Error
		}
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return p
}

func (p *providerTablesTest) tables() []string {
	return []string{p.prefix + "directory_memberships", p.prefix + "directory_users", p.prefix + "sessions", p.prefix + "session_locks"}
}

// createTables stands in for the provider creating its tables during a sign-in.
func (p *providerTablesTest) createTables() {
	p.t.Helper()
	for _, table := range p.tables() {
		require.NoError(p.t, p.db.Exec("CREATE TABLE IF NOT EXISTS "+table+" (id TEXT)").Error)
	}
}

func (p *providerTablesTest) existingTables() []string {
	p.t.Helper()
	var existing []string
	for _, table := range p.tables() {
		if p.db.Migrator().HasTable(table) {
			existing = append(existing, table)
		}
	}
	return existing
}

func TestUnstageDropsTheTablesOfAStagedProvider(t *testing.T) {
	p := newProviderTablesTest(t)
	p.activateOtherProvider()
	require.Empty(t, p.apply(v1.ProviderDesiredStateStaged, oidcSettings()))

	// The verification sign-in ran through the staged provider, which created its tables.
	p.createTables()

	require.Empty(t, p.apply(v1.ProviderDesiredStateUnstaged, nil))
	assert.Empty(t, p.existingTables())
	assert.Nil(t, p.credential(system.ReplacementAuthProviderCredentialContext))
}

func TestUnstageKeepsTheTablesOfAPromotedProviderWhileTheOutgoingOneIsStillConfigured(t *testing.T) {
	p := newProviderTablesTest(t)
	p.activateOtherProvider()
	require.Empty(t, p.apply(v1.ProviderDesiredStateStaged, oidcSettings()))

	// A switch interrupted after promoting the staged credential, before the outgoing provider lost its own: both
	// providers hold an active credential, and the dispatcher may still report the outgoing one as configured.
	require.NoError(t, p.gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: oktaProviderName,
		Name:    oktaProviderName,
		Secrets: oidcSettings(),
	}))
	p.createTables()

	require.Empty(t, p.apply(v1.ProviderDesiredStateUnstaged, nil))
	assert.ElementsMatch(t, p.tables(), p.existingTables())
}

func TestUnstageKeepsTheTablesOfTheConfiguredProvider(t *testing.T) {
	p := newProviderTablesTest(t)
	require.Empty(t, p.apply(v1.ProviderDesiredStateConfigured, oidcSettings()))
	p.createTables()

	// A switch interrupted after promoting the staged credential leaves the provider configured and still staged. Its
	// tables hold the live sessions and recorded groups.
	require.NoError(t, p.gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: system.ReplacementAuthProviderCredentialContext,
		Name:    oktaProviderName,
		Secrets: oidcSettings(),
	}))
	require.Empty(t, p.apply(v1.ProviderDesiredStateUnstaged, nil))
	assert.ElementsMatch(t, p.tables(), p.existingTables())
	assert.True(t, p.status().Configured)
}

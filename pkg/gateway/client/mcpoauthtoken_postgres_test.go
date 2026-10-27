package client

import (
	"context"
	"os"
	"strings"
	"testing"
	"uuid"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func TestReplaceMCPOAuthTokenConcurrentPostgres(t *testing.T) {
	dsn := os.Getenv(postgresUserLimitTestDSNEnv)
	if dsn == "" {
		t.Skipf("set %s to a PostgreSQL URL whose user can create schemas", postgresUserLimitTestDSNEnv)
	}

	admin := newPostgresUserLimitTestDB(t, dsn)
	schema := "obot_oauth_scopes_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	require.NoError(t, admin.WithContext(t.Context()).Exec("CREATE SCHEMA "+schema).Error)
	t.Cleanup(func() {
		require.NoError(t, admin.WithContext(context.Background()).Exec("DROP SCHEMA "+schema+" CASCADE").Error)
	})

	scopedDSN := postgresUserLimitTestDSN(t, dsn, schema)
	first := &Client{db: newPostgresUserLimitTestDB(t, scopedDSN)}
	second := &Client{db: newPostgresUserLimitTestDB(t, scopedDSN)}
	require.NoError(t, first.db.WithContext(t.Context()).AutoMigrate(&types.MCPOAuthToken{}))

	for _, test := range []struct {
		name   string
		scopes []string
	}{
		{
			name: "first grant",
		},
		{
			name:   "scope upgrades",
			scopes: []string{"read"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			conf := oauth2.Config{
				Endpoint: oauth2.Endpoint{
					AuthURL:  "https://provider.test/authorize",
					TokenURL: "https://provider.test/token",
				},
				Scopes: test.scopes,
			}
			const mcpURL = "https://provider.test/mcp"
			if len(test.scopes) > 0 {
				require.NoError(t, first.ReplaceMCPOAuthToken(t.Context(), "user", test.name, mcpURL, "initial", &conf, &oauth2.Token{AccessToken: "initial"}))
			}

			type outcome struct {
				scope string
				err   error
			}

			start := make(chan struct{})
			results := make(chan outcome, 2)
			for i, client := range []*Client{first, second} {
				go func() {
					scope := []string{"write", "admin"}[i]
					candidate := conf
					candidate.Scopes = append([]string{scope}, conf.Scopes...)
					<-start
					err := client.ReplaceMCPOAuthToken(t.Context(), "user", test.name, mcpURL, scope, &candidate, &oauth2.Token{AccessToken: scope})
					results <- outcome{
						scope: scope,
						err:   err,
					}
				}()
			}

			close(start)
			a, b := <-results, <-results
			if a.err != nil {
				a, b = b, a
			}

			require.NoError(t, a.err)
			require.ErrorContains(t, b.err, "start a new authorization")
			stored, err := first.GetMCPOAuthToken(t.Context(), "user", test.name, mcpURL)
			require.NoError(t, err)
			require.Equal(t, a.scope, stored.AccessToken)
			require.Equal(t, strings.Join(append([]string{a.scope}, test.scopes...), " "), stored.Scopes)
		})
	}
}

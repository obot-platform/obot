package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/api"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	"github.com/obot-platform/obot/pkg/gitcredential"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestConvertGitCredentialDoesNotExposeToken(t *testing.T) {
	converted := convertGitCredential(v1.GitCredential{
		ObjectMeta: metav1.ObjectMeta{Name: "gc1-test", Namespace: system.DefaultNamespace},
		Spec: v1.GitCredentialSpec{
			DisplayName: "Shared GitHub",
			Host:        "github.com",
		},
	}, true, true)

	assert.Equal(t, "gc1-test", converted.ID)
	assert.Equal(t, "Shared GitHub", converted.DisplayName)
	assert.Equal(t, "github.com", converted.Host)
	assert.True(t, converted.TokenConfigured)
	assert.True(t, converted.InUse)

	response, err := json.Marshal(converted)
	require.NoError(t, err)
	assert.NotContains(t, string(response), `"token":`)
	assert.Contains(t, string(response), `"inUse":true`)

	converted.InUse = false
	response, err = json.Marshal(converted)
	require.NoError(t, err)
	assert.Contains(t, string(response), `"inUse":false`)
}

func TestReadGitCredentialManifestTrimsToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/git-credentials", strings.NewReader(`{"displayName":"Shared GitHub","host":"github.com","token":"  shared-token  "}`))
	manifest, host, err := readGitCredentialManifest(api.Context{Request: req})
	require.NoError(t, err)
	assert.Equal(t, "github.com", host)
	assert.Equal(t, "shared-token", manifest.Token)

	req = httptest.NewRequest(http.MethodPost, "/api/git-credentials", strings.NewReader(`{"displayName":"Shared GitHub","host":"github.com","token":"   "}`))
	manifest, _, err = readGitCredentialManifest(api.Context{Request: req})
	require.NoError(t, err)
	assert.Empty(t, manifest.Token)
}

func TestGitCredentialReferences(t *testing.T) {
	storage := newFakeStorage(t,
		&v1.SkillRepository{
			ObjectMeta: metav1.ObjectMeta{Name: "skills", Namespace: system.DefaultNamespace},
			Spec:       v1.SkillRepositorySpec{GitCredentialID: "gc1-test"},
		},
		&v1.MCPCatalog{
			ObjectMeta: metav1.ObjectMeta{Name: "catalog", Namespace: system.DefaultNamespace},
			Spec: v1.MCPCatalogSpec{SourceURLGitCredentialIDs: map[string]string{
				"https://github.com/org/catalog": "gc1-test",
				"https://github.com/org/other":   "gc1-test",
			}},
		},
		&v1.SystemMCPCatalog{
			ObjectMeta: metav1.ObjectMeta{Name: "system-catalog", Namespace: system.DefaultNamespace},
			Spec: v1.SystemMCPCatalogSpec{SourceURLGitCredentialIDs: map[string]string{
				"https://github.com/org/system-catalog": "gc1-test",
				"https://github.com/org/system-other":   "gc1-test",
			}},
		},
	)
	req := httptest.NewRequest(http.MethodDelete, "/api/git-credentials/gc1-test", nil)
	referencesByCredential, err := gitcredential.ReferencesByCredential(req.Context(), storage, system.DefaultNamespace)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{
		"skill repository skills",
		"MCP catalog catalog",
		"system MCP catalog system-catalog",
	}, referencesByCredential["gc1-test"])

	references, err := gitCredentialReferences(api.Context{Request: req, Storage: storage}, "gc1-test")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{
		"skill repository skills",
		"MCP catalog catalog",
		"system MCP catalog system-catalog",
	}, references)
}

func TestCatalogSharedCredentialMapHelpers(t *testing.T) {
	values := map[string]string{"github.com/org/repo": "gc1-test"}
	remapCatalogSourceValues(
		[]string{"github.com/org/repo"},
		[]string{"https://github.com/org/repo"},
		values,
	)
	assert.Equal(t, map[string]string{"https://github.com/org/repo": "gc1-test"}, values)

	tokens := map[string]string{
		"https://github.com/org/repo":  "old-token",
		"https://github.com/org/other": "one-off-token",
	}
	removeSharedCredentialTokens(tokens, values)
	assert.Equal(t, map[string]string{"https://github.com/org/other": "one-off-token"}, tokens)
}

func TestValidateCatalogGitCredentials(t *testing.T) {
	const sourceURL = "https://github.com/obot-platform/catalog"

	gatewayClient := newTestGitCredentialGatewayClient(t)
	req := api.Context{
		Request: httptest.NewRequest(http.MethodPut, "/api/mcp-catalogs/default", nil),
		Storage: newFakeStorage(t,
			&v1.GitCredential{
				ObjectMeta: metav1.ObjectMeta{Name: "gc1-github", Namespace: system.DefaultNamespace},
				Spec:       v1.GitCredentialSpec{Host: "github.com"},
			},
			&v1.GitCredential{
				ObjectMeta: metav1.ObjectMeta{Name: "gc1-gitlab", Namespace: system.DefaultNamespace},
				Spec:       v1.GitCredentialSpec{Host: "gitlab.com"},
			},
			&v1.GitCredential{
				ObjectMeta: metav1.ObjectMeta{Name: "gc1-empty", Namespace: system.DefaultNamespace},
				Spec:       v1.GitCredentialSpec{Host: "github.com"},
			},
		),
		GatewayClient: gatewayClient,
	}
	require.NoError(t, gitcredential.Store(t.Context(), gatewayClient, "gc1-github", "shared-token"))

	t.Run("normalizes valid references", func(t *testing.T) {
		references := map[string]string{
			sourceURL:                          "  gc1-github  ",
			"https://github.com/obsolete/repo": "   ",
		}

		require.NoError(t, validateCatalogGitCredentials(req, []string{sourceURL}, references))
		assert.Equal(t, map[string]string{sourceURL: "gc1-github"}, references)
	})

	t.Run("rejects reference for unknown source", func(t *testing.T) {
		err := validateCatalogGitCredentials(req, []string{sourceURL}, map[string]string{
			"https://github.com/unknown/catalog": "gc1-github",
		})
		require.ErrorContains(t, err, "unknown source URL")
	})

	t.Run("rejects credential for another host", func(t *testing.T) {
		err := validateCatalogGitCredentials(req, []string{sourceURL}, map[string]string{
			sourceURL: "gc1-gitlab",
		})
		require.ErrorContains(t, err, "cannot be used with source host")
	})

	t.Run("rejects credential without token", func(t *testing.T) {
		err := validateCatalogGitCredentials(req, []string{sourceURL}, map[string]string{
			sourceURL: "gc1-empty",
		})
		require.ErrorContains(t, err, "failed to reveal Git credential")
	})
}

func newTestGitCredentialGatewayClient(t *testing.T) *gatewayclient.Client {
	t.Helper()
	storageServices, err := sservices.New(sservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	db, err := gatewaydb.New(storageServices.DB.DB, storageServices.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate())
	client := gatewayclient.New(t.Context(), db, nil, nil, nil, nil, nil, time.Hour, 10, 90, 90, true)
	t.Cleanup(func() { _ = client.Close() })
	return client
}

package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
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
	}, true)

	assert.Equal(t, "gc1-test", converted.ID)
	assert.Equal(t, "Shared GitHub", converted.DisplayName)
	assert.Equal(t, "github.com", converted.Host)
	assert.True(t, converted.TokenConfigured)

	response, err := json.Marshal(converted)
	require.NoError(t, err)
	assert.NotContains(t, string(response), `"token":`)
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
			}},
		},
		&v1.SystemMCPCatalog{
			ObjectMeta: metav1.ObjectMeta{Name: "system-catalog", Namespace: system.DefaultNamespace},
			Spec: v1.SystemMCPCatalogSpec{SourceURLGitCredentialIDs: map[string]string{
				"https://github.com/org/system-catalog": "gc1-test",
			}},
		},
	)
	req := httptest.NewRequest(http.MethodDelete, "/api/git-credentials/gc1-test", nil)

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

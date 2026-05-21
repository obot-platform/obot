package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestListAllowedSecretsAllowsOwner(t *testing.T) {
	handler := NewMCPSecretBindingHandler(
		mcp.RuntimeBackendKubernetes,
		newCreateServerSecretBindingK8sClient(t, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "source-secret",
				Namespace: system.DefaultNamespace,
				Labels:    map[string]string{mcp.DefaultSecretBindingAllowedLabel: "true"},
			},
			Data: map[string][]byte{"token": []byte("secret-token")},
		}),
		system.DefaultNamespace,
		mcp.DefaultSecretBindingAllowedLabel,
	)
	req := httptest.NewRequest(http.MethodGet, "/api/mcp-secret-bindings/secrets", nil)
	rec := httptest.NewRecorder()

	err := handler.ListAllowedSecrets(api.Context{
		ResponseWriter: rec,
		Request:        req,
		User:           testUserWithRole("owner-1", types.RoleOwner.Groups()...),
	})

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var result types.MCPAllowedSecretBindingTargetList
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	assert.Equal(t, []types.MCPAllowedSecretBindingTarget{{Name: "source-secret", Keys: []string{"token"}}}, result.Items)
}

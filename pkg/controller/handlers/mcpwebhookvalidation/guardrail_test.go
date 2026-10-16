package mcpwebhookvalidation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type fakeGuardrails struct {
	guardrails map[string]gatewaytypes.Guardrail
	secrets    map[string]map[string]string
}

type recordingResponse struct {
	router.Response
	retryAfter time.Duration
}

func (f fakeGuardrails) GetGuardrail(_ context.Context, id string) (*gatewaytypes.Guardrail, error) {
	p, ok := f.guardrails[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return &p, nil
}

func (f fakeGuardrails) GuardrailSecrets(_ context.Context, id string) (map[string]string, error) {
	return f.secrets[id], nil
}

func (r *recordingResponse) RetryAfter(d time.Duration) { r.retryAfter = d }

func TestGuardrailFilterHasNoSystemServer(t *testing.T) {
	guardrails := fakeGuardrails{
		guardrails: map[string]gatewaytypes.Guardrail{
			"secrets": {ID: "secrets", Type: "secrets", Config: json.RawMessage(`{}`)},
			// Lakera needs an API key, which isn't set.
			"lakera": {ID: "lakera", Type: "lakera", Config: json.RawMessage(`{}`)},
		},
	}

	tests := []struct {
		name           string
		guardrailIDs   []string
		wantConfigured bool
	}{
		{
			name:           "guardrails exist",
			guardrailIDs:   []string{"secrets"},
			wantConfigured: true,
		},
		{
			name:         "a guardrail was deleted",
			guardrailIDs: []string{"secrets", "gone"},
		},
		{
			name:         "a guardrail can't be used",
			guardrailIDs: []string{"lakera"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := &v1.MCPWebhookValidation{}
			filter.Name = "mwv1guardrail"
			filter.Namespace = system.DefaultNamespace
			filter.Spec.Manifest = types.MCPWebhookValidationManifest{
				Name:      "No secrets",
				Guardrail: &types.MCPGuardrailFilterConfig{GuardrailIDs: tt.guardrailIDs, Action: types.MCPGuardrailActionBlock},
			}
			// A server left from before the filter became a guardrail filter.
			leftover := &v1.SystemMCPServer{Name: system.SystemMCPServerPrefix + filter.Name, Namespace: filter.Namespace}
			client := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(filter, leftover).Build()

			resp := &recordingResponse{}
			if err := (&Handler{guardrails: guardrails}).EnsureSystemServer(router.Request{
				Ctx:       t.Context(),
				Client:    client,
				Object:    filter,
				Namespace: filter.Namespace,
				Name:      filter.Name,
			}, resp); err != nil {
				t.Fatal(err)
			}

			if filter.Status.Configured != tt.wantConfigured {
				t.Fatalf("configured = %v, want %v", filter.Status.Configured, tt.wantConfigured)
			}
			if resp.retryAfter == 0 {
				t.Fatal("a guardrail filter's status should be refreshed")
			}
			var servers v1.SystemMCPServerList
			if err := client.List(t.Context(), &servers); err != nil {
				t.Fatal(err)
			}
			if len(servers.Items) != 0 {
				t.Fatalf("a guardrail filter must not have a system MCP server: %#v", servers.Items)
			}
			err := client.Get(t.Context(), router.Key(filter.Namespace, leftover.Name), &v1.SystemMCPServer{})
			if !apierrors.IsNotFound(err) {
				t.Fatalf("expected the leftover server to be deleted, got %v", err)
			}
		})
	}
}

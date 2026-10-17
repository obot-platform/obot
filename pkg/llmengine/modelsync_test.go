package llmengine

import (
	"context"
	"net/http"
	"testing"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// TestTriggerModelSyncRetriesConflicts checks that a conflicting write, as when the provider
// controller updates the ModelProvider at the same time, is retried instead of returned.
func TestTriggerModelSyncRetriesConflicts(t *testing.T) {
	mp := ModelProviderResource()
	conflicts := 2
	client := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(mp).
		WithInterceptorFuncs(interceptor.Funcs{Update: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.UpdateOption) error {
			if conflicts > 0 {
				conflicts--
				return apierrors.NewConflict(schema.GroupResource{Resource: "modelproviders"}, obj.GetName(), nil)
			}
			return c.Update(ctx, obj, opts...)
		}}).
		Build()

	if err := TriggerModelSync(t.Context(), client); err != nil {
		t.Fatalf("TriggerModelSync: %v", err)
	}
	var got v1.ModelProvider
	if err := client.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: system.LLMGatewayModelProvider}, &got); err != nil {
		t.Fatal(err)
	}
	if got.Annotations[v1.ModelProviderSyncAnnotation] != "true" {
		t.Fatalf("sync annotation not set: %v", got.Annotations)
	}
	if conflicts != 0 {
		t.Fatalf("expected both conflicts to be retried, %d left", conflicts)
	}
}

func TestRemoveAutoDetectedKeys(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-from-env-0123456789")
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-from-env-0123456789")
	e, ctx := startTestEngine(t)

	// The engine configured providers from the environment when it started with none, and
	// they were removed.
	var resp struct {
		Providers []struct {
			Name string `json:"name"`
		} `json:"providers"`
	}
	if err := e.Do(ctx, http.MethodGet, "/api/providers", nil, &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Providers) != 0 {
		t.Fatalf("expected the providers the engine configured from the environment to be removed, got %v", resp.Providers)
	}

	// A provider with other keys keeps them.
	if err := e.Do(ctx, http.MethodPost, "/api/providers", map[string]any{"provider": "openai"}, nil); err != nil {
		t.Fatal(err)
	}
	for _, key := range []map[string]any{
		{"name": "OPENAI_API_KEY_auto_detected", "value": "env.OPENAI_API_KEY", "models": []string{"*"}, "weight": 1},
		{"name": "env-OPENAI_API_KEY", "value": map[string]any{"value": "sk-from-env-0123456789", "type": "plain_text"}, "models": []string{"*"}, "weight": 1},
	} {
		if err := e.Do(ctx, http.MethodPost, "/api/providers/openai/keys", key, nil); err != nil {
			t.Fatal(err)
		}
	}
	e.removeAutoDetectedKeys(ctx)
	keys, err := e.providerKeys(ctx, "openai")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0]["name"] != "env-OPENAI_API_KEY" {
		t.Fatalf("expected only Obot's key from the environment to be left, got %v", keys)
	}
}

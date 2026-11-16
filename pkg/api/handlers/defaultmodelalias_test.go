package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDefaultModelAliasRejectsRemovedNames(t *testing.T) {
	for _, name := range []string{"vision", "image-generation", "text-embedding"} {
		for _, method := range []string{http.MethodPost, http.MethodPut} {
			t.Run(name+"/"+method, func(t *testing.T) {
				existing := &v1.DefaultModelAlias{
					Name:      "llm",
					Namespace: system.DefaultNamespace,
					Spec:      v1.DefaultModelAliasSpec{Manifest: types.DefaultModelAliasManifest{Alias: "llm", Model: "m1-target"}},
				}
				c := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(existing).Build()
				body, err := json.Marshal(types.DefaultModelAliasManifest{Alias: name, Model: "m1-target"})
				require.NoError(t, err)
				request := httptest.NewRequest(method, "/api/default-model-aliases/llm", bytes.NewReader(body))
				request.SetPathValue("id", "llm")
				req := api.Context{ResponseWriter: httptest.NewRecorder(), Request: request, Storage: storage.Client(c)}
				h := NewDefaultModelAliasHandler()
				if method == http.MethodPost {
					err = h.Create(req)
				} else {
					err = h.Update(req)
				}
				require.True(t, apierrors.IsBadRequest(err), "%v", err)
				require.Contains(t, err.Error(), "use a concrete gateway model name")
				var aliases v1.DefaultModelAliasList
				require.NoError(t, c.List(t.Context(), &aliases))
				require.Len(t, aliases.Items, 1)
				require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(existing), existing))
				require.Equal(t, "llm", existing.Spec.Manifest.Alias)
			})
		}
	}
}

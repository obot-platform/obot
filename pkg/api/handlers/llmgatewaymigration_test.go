package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGatewayMigrationInitializedWire(t *testing.T) {
	storage := newFakeStorage(t)
	handler, store := newAdminTestHandler(t, storage)
	mux := newAdminTestServer(t, handler, storage, nil)
	mux.HandleFunc("GET /api/llm-gateway/migration", handler.Migration)
	for _, tc := range []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "missing",
			want: `{"initialized":false,"sources":{},"mappings":{}}`,
		},
		{
			name: "old mapping only",
			raw:  `{"mappings":{"old":"target"}}`,
			want: `{"initialized":false,"sources":{},"mappings":{"old":"target"}}`,
		},
		{
			name: "initialized sanitized outcome",
			raw:  `{"initialized":true,"sources":{"old":{"name":"OpenAI","provider":"openai","status":"error","secrets":{"key":"hidden"}}},"credentials":"hidden"}`,
			want: `{"initialized":true,"sources":{"old":{"name":"OpenAI","provider":"openai","status":"error"}},"mappings":{}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.raw != "" {
				require.NoError(t, store.MutateSettings(t.Context(), func(s map[string]json.RawMessage) error {
					s["legacy-provider-migration"] = json.RawMessage(tc.raw)
					return nil
				}))
			}
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/llm-gateway/migration", nil))
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			require.JSONEq(t, tc.want, response.Body.String())
		})
	}
}

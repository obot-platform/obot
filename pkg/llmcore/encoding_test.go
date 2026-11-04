package llmcore

import (
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/stretchr/testify/require"
)

func TestEncodingFailure(t *testing.T) {
	// A non-finite provider/plugin value must never become a successful {} response.
	value := map[string]any{"score": math.NaN()}
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, value)
	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.Contains(t, w.Body.String(), `"type":"api_error"`)

	w = httptest.NewRecorder()
	require.Error(t, writeEvent(w, "response.completed", value))
	require.Empty(t, w.Body.String(), "failed encoding must not write a partial SSE event")
}

func TestErrorMessagePreservesProviderText(t *testing.T) {
	for _, tt := range []struct {
		message string
		want    string
	}{
		{
			message: "Bifrost: provider unavailable",
			want:    "gateway: provider unavailable",
		},
		{
			message: "model bifrost-v2 is unavailable",
			want:    "model bifrost-v2 is unavailable",
		},
	} {
		t.Run(tt.message, func(t *testing.T) {
			_, _, got := errorParts(&schemas.BifrostError{Error: &schemas.ErrorField{Message: tt.message}})
			require.Equal(t, tt.want, got)
		})
	}
}

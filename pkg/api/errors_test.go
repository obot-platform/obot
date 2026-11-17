package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/require"
)

func TestErrHTTPWithReasonsIsAnErrHTTP(t *testing.T) {
	err := fmt.Errorf("wrapped: %w", NewErrHTTPWithReasons(http.StatusBadRequest, "set the URL", []types.AgentReadinessReason{{Code: types.AgentReadinessConfigurationRequired}}))

	var httpErr *types.ErrHTTP
	require.ErrorAs(t, err, &httpErr)
	require.Equal(t, http.StatusBadRequest, httpErr.Code)
	require.Equal(t, "set the URL", httpErr.Message)

	var withReasons *ErrHTTPWithReasons
	require.True(t, errors.As(err, &withReasons))
	require.Len(t, withReasons.Reasons, 1)
}

func TestWriteErrorWithReasons(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteErrorWithReasons(rec, NewErrHTTPWithReasons(http.StatusBadRequest, "needs __url", []types.AgentReadinessReason{{
		ComponentID: "github",
		Code:        types.AgentReadinessConfigurationRequired,
		Field:       "__url",
		ProfileID:   "vp1agents",
	}}))

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	require.JSONEq(t, `{"error":"needs __url","reasons":[{"componentID":"github","code":"configuration_required","field":"__url","profileID":"vp1agents"}]}`, rec.Body.String())
}

func TestWriteErrorWithReasonsAlwaysHasReasonsList(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteErrorWithReasons(rec, NewErrHTTPWithReasons(http.StatusConflict, "no reasons", nil))

	var body ErrorBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "no reasons", body.Error)
	require.NotNil(t, body.Reasons)
	require.Empty(t, body.Reasons)
}

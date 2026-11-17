package api

import (
	"encoding/json"
	"net/http"

	"github.com/obot-platform/obot/apiclient/types"
)

// ErrHTTPWithReasons is an *types.ErrHTTP that also tells a client what to fix, as structured reasons. The server writes it
// as a JSON body, {"error": <message>, "reasons": [...]}, the shape the MCP gateway gives a 409 to an agent that cannot
// use a vMCP (types.AgentReadinessReason). Clients that read only the message find it in "error", where they already
// look for it in a JSON error body. A reason never carries a configuration value.
type ErrHTTPWithReasons struct {
	*types.ErrHTTP
	Reasons []types.AgentReadinessReason
}

// ErrorBody is the JSON body of an ErrHTTPWithReasons.
type ErrorBody struct {
	Error   string                       `json:"error"`
	Reasons []types.AgentReadinessReason `json:"reasons"`
}

// NewErrHTTPWithReasons returns an error that is code and message, with reasons.
func NewErrHTTPWithReasons(code int, message string, reasons []types.AgentReadinessReason) *ErrHTTPWithReasons {
	return &ErrHTTPWithReasons{ErrHTTP: types.NewErrHTTP(code, message), Reasons: reasons}
}

// Error is declared on the pointer so that only a pointer is an error, and errors.As finds it as one.
func (e *ErrHTTPWithReasons) Error() string {
	return e.ErrHTTP.Error()
}

// Unwrap lets callers that look for the plain *types.ErrHTTP, such as status handling, find it.
func (e *ErrHTTPWithReasons) Unwrap() error {
	return e.ErrHTTP
}

// WriteErrorWithReasons writes err as its JSON body, with its status.
func WriteErrorWithReasons(w http.ResponseWriter, err *ErrHTTPWithReasons) {
	reasons := err.Reasons
	if reasons == nil {
		reasons = []types.AgentReadinessReason{}
	}
	body, marshalErr := json.Marshal(ErrorBody{Error: err.Message, Reasons: reasons})
	if marshalErr != nil {
		// The body is only strings, so this cannot happen; the message alone still tells the client what is wrong.
		http.Error(w, err.Message, err.Code)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(err.Code)
	_, _ = w.Write(append(body, '\n'))
}

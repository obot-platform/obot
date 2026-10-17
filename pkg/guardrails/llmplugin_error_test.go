package guardrails

import (
	"errors"
	"strings"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
)

func TestProviderError(t *testing.T) {
	status := func(code int) *int { return &code }
	str := func(s string) *string { return &s }
	allow := func(b bool) *bool { return &b }

	tests := []struct {
		name          string
		err           *schemas.BifrostError
		wantFallbacks bool
	}{
		{
			name: "invalid request",
			err: &schemas.BifrostError{StatusCode: status(400), Error: &schemas.ErrorField{
				Type: str("invalid_request_error"), Code: str("invalid_image_format"), Message: "You uploaded an unsupported image.",
			}},
			wantFallbacks: false,
		},
		{
			name:          "server error",
			err:           &schemas.BifrostError{StatusCode: status(503), Error: &schemas.ErrorField{Type: str("server_error"), Message: "overloaded"}},
			wantFallbacks: true,
		},
		{
			name: "model not found",
			err: &schemas.BifrostError{StatusCode: status(404), Error: &schemas.ErrorField{
				Type: str("invalid_request_error"), Code: str("model_not_found"), Message: "The model does not exist",
			}},
			wantFallbacks: true,
		},
		{
			name: "rate limited",
			err: &schemas.BifrostError{StatusCode: status(429), Error: &schemas.ErrorField{
				Type: str("rate_limit_error"), Message: "Rate limit reached",
			}},
			wantFallbacks: true,
		},
		{
			// An access refusal says to go on to the next fallback, whatever its status.
			name:          "access refusal",
			err:           &schemas.BifrostError{StatusCode: status(400), AllowFallbacks: allow(true), Error: &schemas.ErrorField{Type: str("permission_denied"), Message: "not available"}},
			wantFallbacks: true,
		},
		{
			// No key serves the model: the gateway refused it before any provider saw it, and
			// a fallback on another provider or key may serve it.
			name: "no key supports the model",
			err: &schemas.BifrostError{
				Type:       str(schemas.NoKeySupportsModel),
				StatusCode: status(400),
				Error: &schemas.ErrorField{
					Type: str(schemas.NoKeySupportsModel), Message: "no keys found that support model: claude-haiku-4-5",
				},
				ExtraFields: schemas.BifrostErrorExtraFields{ErrorType: schemas.ErrorTypeCallerModelNotAvailable},
			},
			wantFallbacks: true,
		},
		{
			// The engine's own conversion can refuse a request one provider can't take; another
			// provider may.
			name:          "engine refusal",
			err:           &schemas.BifrostError{IsBifrostError: true, StatusCode: status(400), Error: &schemas.ErrorField{Type: str("invalid_request_error"), Message: "max_tokens too low for thinking"}},
			wantFallbacks: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := providerError(tt.err)
			if fallbacks := got.AllowFallbacks == nil || *got.AllowFallbacks; fallbacks != tt.wantFallbacks {
				t.Fatalf("fallbacks allowed = %v, want %v", fallbacks, tt.wantFallbacks)
			}
		})
	}

	if providerError(nil) != nil {
		t.Fatal("no error became one")
	}
}

func TestProviderErrorTimeout(t *testing.T) {
	status, errType := 504, schemas.RequestTimedOut
	err := &schemas.BifrostError{
		IsBifrostError: true,
		StatusCode:     &status,
		Error: &schemas.ErrorField{
			Type:    &errType,
			Message: schemas.ErrProviderRequestTimedOut,
			Error:   errors.New("fasthttp: timeout"),
		},
	}
	err.ExtraFields.RoutingInfo.Provider = "openai"
	got := providerError(err)
	if got.Error.Error != nil {
		t.Fatalf("the engine's internal error %v is still reported", got.Error.Error)
	}
	msg := got.Error.Message
	if !strings.Contains(msg, "timed out") || !strings.Contains(msg, `"openai"`) || strings.Contains(msg, "network_config") || strings.Contains(msg, "default is 300") {
		t.Fatalf("message %q", msg)
	}
}

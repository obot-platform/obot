package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
)

func TestDisplayLicenseKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		licenseKey     string
		canViewPartial bool
		want           string
	}{
		{
			name:           "empty key",
			licenseKey:     "",
			canViewPartial: true,
			want:           "",
		},
		{
			name:           "non admin masks key",
			licenseKey:     "keygen/abc123invalid",
			canViewPartial: false,
			want:           licenseKeyMask,
		},
		{
			name:           "admin shows suffix",
			licenseKey:     "keygen/abc123j13lasds",
			canViewPartial: true,
			want:           "****j13lasds",
		},
		{
			name:           "admin short key is fully masked",
			licenseKey:     "shortkey",
			canViewPartial: true,
			want:           licenseKeyMask,
		},
		{
			name:           "trims whitespace",
			licenseKey:     "  keygen/license-key  ",
			canViewPartial: false,
			want:           licenseKeyMask,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := displayLicenseKey(tt.licenseKey, tt.canViewPartial); got != tt.want {
				t.Fatalf("displayLicenseKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCheckLicenseCooldown(t *testing.T) {
	t.Parallel()

	handler := NewLicenseHandler(nil)
	handler.lastManualLicenseCheck = time.Now()

	recorder := httptest.NewRecorder()
	err := handler.CheckLicense(api.Context{
		ResponseWriter: recorder,
		Request:        httptest.NewRequest(http.MethodPost, "/api/license", nil),
	})

	var errHTTP *apitypes.ErrHTTP
	if !errors.As(err, &errHTTP) {
		t.Fatalf("expected ErrHTTP, got %T: %v", err, err)
	}
	if errHTTP.Code != http.StatusTooManyRequests {
		t.Fatalf("ErrHTTP.Code = %d, want %d", errHTTP.Code, http.StatusTooManyRequests)
	}
	if got := recorder.Header().Get("Retry-After"); got == "" {
		t.Fatal("expected Retry-After header")
	}
}

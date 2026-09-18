package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/license"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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

func TestDeleteLicenseReturnsCommunityFallbackStatus(t *testing.T) {
	provider := &fakeCommunityLicenseProvider{
		key:                "keygen/enterprise-12345678",
		valid:              true,
		entitlements:       []string{license.EnterpriseEntitlement},
		removeKey:          "keygen/community-87654321",
		removeEntitlements: []string{license.CommunityEntitlement},
	}
	handler := NewLicenseHandler(provider, nil)
	recorder := httptest.NewRecorder()

	err := handler.Delete(api.Context{
		ResponseWriter: recorder,
		Request:        httptest.NewRequest(http.MethodDelete, "/api/license", nil),
		User: &user.DefaultInfo{
			Name:   "admin",
			Groups: apitypes.RoleAdmin.Groups(),
		},
	})
	if err != nil {
		t.Fatalf("expected license deletion to succeed: %v", err)
	}

	var status LicenseStatus
	if err := json.NewDecoder(recorder.Body).Decode(&status); err != nil {
		t.Fatalf("failed to decode license status: %v", err)
	}
	if status.LicenseKey != "****87654321" {
		t.Fatalf("license key = %q, want masked Community fallback", status.LicenseKey)
	}
	if status.Source != "database" || !status.Enterprise {
		t.Fatalf("status = %#v, want valid database license", status)
	}
	if len(status.Entitlements) != 1 || status.Entitlements[0] != license.CommunityEntitlement {
		t.Fatalf("entitlements = %v, want [%s]", status.Entitlements, license.CommunityEntitlement)
	}
}

func TestCheckLicenseCooldown(t *testing.T) {
	t.Parallel()

	handler := NewLicenseHandler(nil, nil)
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

func TestCheckLicenseSignalsAfterSuccessfulValidation(t *testing.T) {
	for _, tt := range []struct {
		name         string
		validateErr  error
		wantRevision int64
	}{
		{name: "success", wantRevision: 5},
		{name: "not configured", validateErr: license.ErrNotConfigured, wantRevision: 4},
	} {
		t.Run(tt.name, func(t *testing.T) {
			daemonSync := &v1.ProviderSync{
				Name:      system.ProviderSyncName,
				Namespace: system.DefaultNamespace,
				Spec: v1.ProviderSyncSpec{
					Revisions: map[string]v1.ProviderRevision{
						string(v1.ProviderTypeLicense): {
							ProviderType: v1.ProviderTypeLicense,
							Revision:     4,
						},
					},
				},
			}
			storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(daemonSync).Build()
			handler := NewLicenseHandler(&fakeCommunityLicenseProvider{
				valid:         true,
				validateError: tt.validateErr,
			}, nil)

			err := handler.CheckLicense(api.Context{
				ResponseWriter: httptest.NewRecorder(),
				Request:        httptest.NewRequest(http.MethodPost, "/api/license/check", nil),
				Storage:        storage,
				User:           &user.DefaultInfo{},
			})
			if err != nil {
				t.Fatalf("expected license check to succeed: %v", err)
			}

			var updatedDaemonSync v1.ProviderSync
			if err := storage.Get(t.Context(), kclient.ObjectKey{
				Namespace: system.DefaultNamespace,
				Name:      system.ProviderSyncName,
			}, &updatedDaemonSync); err != nil {
				t.Fatalf("expected provider daemon sync to be available: %v", err)
			}
			licenseRevision := updatedDaemonSync.Spec.Revisions[string(v1.ProviderTypeLicense)]
			if licenseRevision.Revision != tt.wantRevision {
				t.Fatalf("license revision = %d, want %d", licenseRevision.Revision, tt.wantRevision)
			}
		})
	}
}

package license

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func runtimeLicenseFile(issuer testLicenseFileIssuer, issued, expiry time.Time, mutations ...func(map[string]any)) string {
	dataset := testLicenseFileDataset()
	setMeta("issued", issued.Format(time.RFC3339))(dataset)
	setAttribute("expiry", expiry.Format(time.RFC3339))(dataset)
	for _, mutate := range mutations {
		mutate(dataset)
	}
	return issuer.signDataset(dataset)
}

func countingKeygen(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()

	var requests atomic.Int32
	keygen := newKeyedKeygen()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		keygen.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	return server, &requests
}

func TestLicenseFilePrecedence(t *testing.T) {
	issuer := newTestLicenseFileIssuer("runtime")
	now := time.Now().Truncate(time.Second)
	file := runtimeLicenseFile(issuer, now.Add(-time.Hour), now.Add(time.Hour))
	foreignFile := runtimeLicenseFile(newTestLicenseFileIssuer("foreign"), now.Add(-time.Hour), now.Add(time.Hour))
	fileEntitlements := []string{EnterpriseEntitlement, "OBOT_ENTERPRISE_250_USERS", EnterpriseAuthProvidersEntitlement, "OBOT_FUTURE_FEATURE"}

	for _, tt := range []struct {
		name             string
		config           Config
		storedFile       string
		storedKey        string
		community        string
		wantErr          error
		wantEntitlements []string
		wantLicenseFile  bool
		wantFileError    bool
		wantNoRequests   bool
	}{
		{
			name:             "configured file in an air gap",
			config:           Config{LicenseFile: file}.WithAirgap(true),
			storedFile:       foreignFile,
			wantEntitlements: fileEntitlements,
			wantLicenseFile:  true,
			wantNoRequests:   true,
		},
		{
			name:    "configured file without an air gap",
			config:  Config{LicenseFile: file},
			wantErr: ErrNotAirgap,
		},
		{
			name:    "configured key that is a file",
			config:  Config{LicenseKey: file},
			wantErr: ErrLicenseFileAsKey,
		},
		{
			name:    "configured key in an air gap",
			config:  Config{LicenseKey: "paid-license"}.WithAirgap(true),
			wantErr: ErrAirgap,
		},
		{
			name:             "stored file in an air gap",
			config:           Config{}.WithAirgap(true),
			storedFile:       file,
			storedKey:        "paid-license",
			community:        "community-license",
			wantEntitlements: fileEntitlements,
			wantLicenseFile:  true,
			wantNoRequests:   true,
		},
		{
			name:            "stored file from an untrusted issuer in an air gap",
			config:          Config{}.WithAirgap(true),
			storedFile:      foreignFile,
			wantLicenseFile: true,
			wantFileError:   true,
			wantNoRequests:  true,
		},
		{
			name:           "stored key in an air gap",
			config:         Config{}.WithAirgap(true),
			storedKey:      "paid-license",
			community:      "community-license",
			wantNoRequests: true,
		},
		{
			name:             "stored file without an air gap",
			storedFile:       file,
			storedKey:        "paid-license",
			wantEntitlements: []string{CommunityEntitlement, EnterpriseEntitlement},
		},
		{
			name:             "configured key",
			config:           Config{LicenseKey: "paid-license"},
			wantEntitlements: []string{CommunityEntitlement, EnterpriseEntitlement},
		},
		{
			name:             "community key",
			community:        "community-license",
			wantEntitlements: []string{CommunityEntitlement},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			server, requests := countingKeygen(t)
			gatewayClient := newTestLicenseGatewayClient(t)
			for propertyKey, value := range map[string]string{
				LicenseFilePropertyKey:         tt.storedFile,
				LicenseKeyPropertyKey:          tt.storedKey,
				CommunityLicenseKeyPropertyKey: tt.community,
			} {
				if value == "" {
					continue
				}
				if _, err := gatewayClient.SetProperty(ctx, propertyKey, value); err != nil {
					t.Fatalf("failed to store %s: %v", propertyKey, err)
				}
			}

			provider, err := newProvider(ctx, gatewayClient, tt.config, server.URL, issuer.verifyKey)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("newProvider() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("newProvider() error = %v", err)
			}
			if err := provider.Validate(ctx); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}

			codes, err := provider.codes(ctx)
			if err != nil || !slices.Equal(codes, tt.wantEntitlements) {
				t.Fatalf("entitlement codes = %v, %v, want %v", codes, err, tt.wantEntitlements)
			}
			licenseFile, _, err := provider.LicenseFile(ctx)
			if err != nil || licenseFile != tt.wantLicenseFile {
				t.Fatalf("LicenseFile() = %v, %v, want %v", licenseFile, err, tt.wantLicenseFile)
			}
			if got := provider.LicenseFileError(); (got != "") != tt.wantFileError {
				t.Fatalf("LicenseFileError() = %q, want an error: %v", got, tt.wantFileError)
			}
			if tt.wantLicenseFile {
				if licenseKey, err := provider.LicenseKey(ctx); err != nil || licenseKey != "" {
					t.Fatalf("LicenseKey() = %q, %v, want none while a license file is in effect", licenseKey, err)
				}
			}
			if got := requests.Load(); tt.wantNoRequests && got != 0 {
				t.Fatalf("Keygen received %d requests, want none", got)
			}
		})
	}
}

func TestConfiguredLicenseFile(t *testing.T) {
	issuer := newTestLicenseFileIssuer("runtime")
	now := time.Now().Truncate(time.Second)
	issued, expiry := now.Add(-time.Hour), now.Add(time.Hour)
	dataset := mustMarshalJSON(testLicenseFileDataset())
	tampered := issuer.envelope(dataset, licenseFileAlgorithm)
	tampered["sig"] = issuer.envelope([]byte("{}"), licenseFileAlgorithm)["sig"]

	// A configured file that can't be read or verified starts the installation unlicensed and says why.
	for _, tt := range []struct {
		name      string
		contents  string
		wantError string
	}{
		{
			name:      "tampered",
			contents:  armorTestLicenseFile(mustMarshalJSON(tampered)),
			wantError: errLicenseFileInvalid.Message,
		},
		{
			name:      "foreign key",
			contents:  runtimeLicenseFile(newTestLicenseFileIssuer("foreign"), issued, expiry),
			wantError: errLicenseFileInvalid.Message,
		},
		{
			name:      "encrypted",
			contents:  armorTestLicenseFile(mustMarshalJSON(issuer.envelope(dataset, "aes-256-gcm+ed25519"))),
			wantError: errLicenseFileInvalid.Message,
		},
		{
			name:      "a path instead of the contents",
			contents:  "/etc/obot/license.lic",
			wantError: errNotALicenseFile.Message,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := newProvider(t.Context(), nil, Config{LicenseFile: tt.contents}.WithAirgap(true), "http://keygen.invalid", issuer.verifyKey)
			if err != nil {
				t.Fatalf("newProvider() error = %v, want the installation to start unlicensed", err)
			}
			if requireValidLicense(t.Context(), t, provider) {
				t.Fatal("a license file that can't be verified is in effect")
			}
			licenseFile, _, err := provider.LicenseFile(t.Context())
			if err != nil || !licenseFile || !provider.LicenseKeyViaConfiguration() {
				t.Fatalf("LicenseFile() = %v, %v, want the configured file still in effect and locked", licenseFile, err)
			}
			if got := provider.LicenseFileError(); !strings.HasPrefix(got, tt.wantError) {
				t.Fatalf("LicenseFileError() = %q, want %q", got, tt.wantError)
			}
		})
	}

	t.Run("verified", func(t *testing.T) {
		provider, err := newProvider(t.Context(), nil, Config{LicenseFile: runtimeLicenseFile(issuer, issued, expiry)}.WithAirgap(true), "http://keygen.invalid", issuer.verifyKey)
		if err != nil {
			t.Fatalf("newProvider() error = %v", err)
		}
		if !requireValidLicense(t.Context(), t, provider) || provider.LicenseFileError() != "" {
			t.Fatalf("configured license file isn't in effect: %s", provider.LicenseFileError())
		}
		if err := provider.SetLicenseFile(t.Context(), runtimeLicenseFile(issuer, issued, expiry)); !errors.Is(err, ErrLicenseKeyViaConfiguration) {
			t.Fatalf("SetLicenseFile() error = %v, want %v", err, ErrLicenseKeyViaConfiguration)
		}
	})

	t.Run("expired", func(t *testing.T) {
		provider, err := newProvider(t.Context(), nil, Config{
			LicenseFile: runtimeLicenseFile(issuer, now.Add(-2*time.Hour), now.Add(-time.Hour)),
		}.WithAirgap(true), "http://keygen.invalid", issuer.verifyKey)
		if err != nil {
			t.Fatalf("newProvider() error = %v", err)
		}
		if requireValidLicense(t.Context(), t, provider) {
			t.Fatal("expired license file is in effect")
		}
		licenseFile, gotExpiry, err := provider.LicenseFile(t.Context())
		if err != nil || !licenseFile || gotExpiry == nil || !gotExpiry.Equal(now.Add(-time.Hour)) {
			t.Fatalf("LicenseFile() = %v, %v, %v, want the expired file", licenseFile, gotExpiry, err)
		}
	})
}

func TestStoredLicenseFileValidity(t *testing.T) {
	ctx := t.Context()
	issuer := newTestLicenseFileIssuer("runtime")
	issued := time.Date(2026, time.November, 13, 0, 0, 0, 0, time.UTC)
	expiry := issued.AddDate(1, 0, 0)
	gatewayClient := newTestLicenseGatewayClient(t)
	if _, err := gatewayClient.SetProperty(ctx, LicenseFilePropertyKey, runtimeLicenseFile(issuer, issued, expiry)); err != nil {
		t.Fatal(err)
	}
	provider, err := newProvider(ctx, gatewayClient, Config{}.WithAirgap(true), "http://keygen.invalid", issuer.verifyKey)
	if err != nil {
		t.Fatalf("newProvider() error = %v", err)
	}

	for _, tt := range []struct {
		name string
		now  time.Time
		want bool
	}{
		{
			name: "issued",
			now:  issued,
			want: true,
		},
		{
			name: "just before expiry",
			now:  expiry.Add(-time.Nanosecond),
			want: true,
		},
		{
			name: "expiry",
			now:  expiry,
			want: false,
		},
	} {
		provider.now = func() time.Time { return tt.now }
		if got := requireValidLicense(ctx, t, provider); got != tt.want {
			t.Errorf("%s: HasValidLicense() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestSetLicenseFile(t *testing.T) {
	ctx := t.Context()
	issuer := newTestLicenseFileIssuer("runtime")
	now := time.Now().Truncate(time.Second)
	file := runtimeLicenseFile(issuer, now.Add(-time.Hour), now.Add(time.Hour))

	t.Run("without an air gap", func(t *testing.T) {
		server := httptest.NewServer(newKeyedKeygen())
		t.Cleanup(server.Close)
		provider, err := newProvider(ctx, newTestLicenseGatewayClient(t), Config{}, server.URL, issuer.verifyKey)
		if err != nil {
			t.Fatal(err)
		}
		if err := provider.SetLicenseFile(ctx, file); !errors.Is(err, ErrNotAirgap) {
			t.Fatalf("SetLicenseFile() error = %v, want %v", err, ErrNotAirgap)
		}
		if err := provider.SetLicenseKey(ctx, file); !errors.Is(err, ErrLicenseFileAsKey) {
			t.Fatalf("SetLicenseKey() error = %v, want %v", err, ErrLicenseFileAsKey)
		}
		if licenseFile, _, err := provider.LicenseFile(ctx); err != nil || licenseFile {
			t.Fatalf("LicenseFile() = %v, %v, want none", licenseFile, err)
		}
	})

	t.Run("air-gapped", func(t *testing.T) {
		var requests atomic.Int32
		airGapped := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			unavailable(w, r)
		}))
		t.Cleanup(airGapped.Close)
		gatewayClient := newTestLicenseGatewayClient(t)
		if _, err := gatewayClient.SetProperty(ctx, LicenseKeyPropertyKey, "paid-license"); err != nil {
			t.Fatal(err)
		}
		replicaOne, err := newProvider(ctx, gatewayClient, Config{}.WithAirgap(true), airGapped.URL, issuer.verifyKey)
		if err != nil {
			t.Fatal(err)
		}
		replicaTwo, err := newProvider(ctx, gatewayClient, Config{}.WithAirgap(true), airGapped.URL, issuer.verifyKey)
		if err != nil {
			t.Fatal(err)
		}

		for _, set := range []func(context.Context, string) error{replicaOne.SetLicenseKey, replicaOne.SetCommunityLicenseKey} {
			if err := set(ctx, "paid-license"); !errors.Is(err, ErrAirgap) {
				t.Fatalf("setting a license key: error = %v, want %v", err, ErrAirgap)
			}
		}
		if requireValidLicense(ctx, t, replicaOne) {
			t.Fatal("a stored license key licenses an air-gapped installation")
		}

		for name, contents := range map[string]string{
			"foreign key": runtimeLicenseFile(newTestLicenseFileIssuer("foreign"), now.Add(-time.Hour), now.Add(time.Hour)),
			"expired":     runtimeLicenseFile(issuer, now.Add(-2*time.Hour), now.Add(-time.Hour)),
		} {
			if _, ok := errors.AsType[*FileError](replicaOne.SetLicenseFile(ctx, contents)); !ok {
				t.Fatalf("%s: SetLicenseFile() didn't refuse the license file", name)
			}
		}

		// Every replica uses the stored file from its next read.
		for _, expiry := range []time.Time{now.Add(time.Hour), now.Add(2 * time.Hour)} {
			if err := replicaOne.SetLicenseFile(ctx, runtimeLicenseFile(issuer, now.Add(-time.Hour), expiry)); err != nil {
				t.Fatalf("SetLicenseFile() error = %v", err)
			}
			licenseFile, gotExpiry, err := replicaTwo.LicenseFile(ctx)
			if err != nil || !licenseFile || gotExpiry == nil || !gotExpiry.Equal(expiry) {
				t.Fatalf("LicenseFile() = %v, %v, %v, want expiry %v", licenseFile, gotExpiry, err, expiry)
			}
		}
		if !requireValidLicense(ctx, t, replicaTwo) {
			t.Fatal("uploaded license file isn't in effect")
		}
		if licenseKey, err := replicaTwo.LicenseKey(ctx); err != nil || licenseKey != "" {
			t.Fatalf("LicenseKey() = %q, %v, want none while a license file is in effect", licenseKey, err)
		}

		// Removing the file leaves the installation unlicensed; the stored key still isn't used.
		if err := replicaOne.RemoveLicenseKey(ctx); err != nil {
			t.Fatalf("RemoveLicenseKey() error = %v", err)
		}
		if licenseFile, _, err := replicaTwo.LicenseFile(ctx); err != nil || licenseFile {
			t.Fatalf("LicenseFile() = %v, %v, want none once the file is removed", licenseFile, err)
		}
		if requireValidLicense(ctx, t, replicaTwo) {
			t.Fatal("an installation is licensed after its license file is removed")
		}
		if got := requests.Load(); got != 0 {
			t.Fatalf("Keygen received %d requests", got)
		}
	})
}

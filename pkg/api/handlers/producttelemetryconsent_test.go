package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	"github.com/obot-platform/obot/pkg/producttelemetry"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
)

func TestProductTelemetryConsentGetStates(t *testing.T) {
	client := newProductTelemetryConsentTestGatewayClient(t)
	consent := producttelemetry.NewConsent(client, false)
	handler := NewProductTelemetryConsentHandler(consent)

	recorder := httptest.NewRecorder()
	err := handler.Get(api.Context{
		ResponseWriter: recorder,
		Request:        httptest.NewRequest(http.MethodGet, "/api/product-telemetry-consent", nil),
	})
	if err != nil {
		t.Fatalf("Get() undecided error = %v", err)
	}
	var response types.ProductTelemetryConsent
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode undecided response: %v", err)
	}
	if response.Consent != nil {
		t.Fatalf("undecided response consent = %t, want nil", *response.Consent)
	}
	if strings.Contains(recorder.Body.String(), `"consent"`) {
		t.Fatalf("undecided response includes consent: %s", recorder.Body.String())
	}

	for _, want := range []bool{false, true} {
		if err := consent.Set(t.Context(), want); err != nil {
			t.Fatalf("Set(%t) error = %v", want, err)
		}

		recorder := httptest.NewRecorder()
		err := handler.Get(api.Context{
			ResponseWriter: recorder,
			Request:        httptest.NewRequest(http.MethodGet, "/api/product-telemetry-consent", nil),
		})
		if err != nil {
			t.Fatalf("Get() after Set(%t) error = %v", want, err)
		}
		var response types.ProductTelemetryConsent
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode response after Set(%t): %v", want, err)
		}
		if response.Consent == nil || *response.Consent != want {
			t.Fatalf("response consent after Set(%t) = %v", want, response.Consent)
		}
	}
}

func TestProductTelemetryConsentUpdateChangesBothDirections(t *testing.T) {
	client := newProductTelemetryConsentTestGatewayClient(t)
	consent := producttelemetry.NewConsent(client, false)
	handler := NewProductTelemetryConsentHandler(consent)

	for _, want := range []bool{true, false} {
		recorder := httptest.NewRecorder()
		err := handler.Update(api.Context{
			ResponseWriter: recorder,
			Request: httptest.NewRequest(
				http.MethodPut,
				"/api/product-telemetry-consent",
				strings.NewReader(`{"consent":`+strconv.FormatBool(want)+`}`),
			),
		})
		if err != nil {
			t.Fatalf("Update(%t) error = %v", want, err)
		}

		var response types.ProductTelemetryConsent
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode Update(%t) response: %v", want, err)
		}
		if response.Consent == nil || *response.Consent != want {
			t.Fatalf("Update(%t) response consent = %v", want, response.Consent)
		}

		stored, err := consent.Get(t.Context())
		if err != nil {
			t.Fatalf("Get() after Update(%t) error = %v", want, err)
		}
		if stored == nil || *stored != want {
			t.Fatalf("stored consent after Update(%t) = %v", want, stored)
		}
	}
}

func TestProductTelemetryConsentUpdateRejectsMalformedInput(t *testing.T) {
	client := newProductTelemetryConsentTestGatewayClient(t)
	consent := producttelemetry.NewConsent(client, false)
	handler := NewProductTelemetryConsentHandler(consent)
	if err := consent.Set(t.Context(), true); err != nil {
		t.Fatalf("Set(true) error = %v", err)
	}

	for _, body := range []string{
		`{`,
		`{}`,
		`{"consent":null}`,
		`{"consent":"true"}`,
	} {
		t.Run(body, func(t *testing.T) {
			err := handler.Update(api.Context{
				ResponseWriter: httptest.NewRecorder(),
				Request: httptest.NewRequest(
					http.MethodPut,
					"/api/product-telemetry-consent",
					strings.NewReader(body),
				),
			})
			var httpErr *types.ErrHTTP
			if !errors.As(err, &httpErr) || httpErr.Code != http.StatusBadRequest {
				t.Fatalf("Update() error = %v, want HTTP 400", err)
			}

			stored, err := consent.Get(t.Context())
			if err != nil {
				t.Fatalf("Get() after rejected update error = %v", err)
			}
			if stored == nil || !*stored {
				t.Fatalf("stored consent after rejected update = %v, want true", stored)
			}
		})
	}
}

func TestProductTelemetryConsentAPIIsNotFoundWhenForceEnabled(t *testing.T) {
	client := newProductTelemetryConsentTestGatewayClient(t)
	persisted := producttelemetry.NewConsent(client, false)
	if err := persisted.Set(t.Context(), false); err != nil {
		t.Fatalf("Set(false) error = %v", err)
	}
	handler := NewProductTelemetryConsentHandler(producttelemetry.NewConsent(client, true))

	err := handler.Get(api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        httptest.NewRequest(http.MethodGet, "/api/product-telemetry-consent", nil),
	})
	var httpErr *types.ErrHTTP
	if !errors.As(err, &httpErr) || httpErr.Code != http.StatusNotFound {
		t.Fatalf("Get() error = %v, want HTTP 404", err)
	}

	err = handler.Update(api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request: httptest.NewRequest(
			http.MethodPut,
			"/api/product-telemetry-consent",
			strings.NewReader(`{"consent":true}`),
		),
	})
	if !errors.As(err, &httpErr) || httpErr.Code != http.StatusNotFound {
		t.Fatalf("Update() error = %v, want HTTP 404", err)
	}

	stored, err := persisted.Get(t.Context())
	if err != nil {
		t.Fatalf("Get() persisted value error = %v", err)
	}
	if stored == nil || *stored {
		t.Fatalf("stored consent = %v, want false", stored)
	}
}

func newProductTelemetryConsentTestGatewayClient(t *testing.T) *gatewayclient.Client {
	t.Helper()

	storageServices, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	if err != nil {
		t.Fatalf("create storage services: %v", err)
	}
	db, err := gatewaydb.New(storageServices.DB.DB, storageServices.DB.SQLDB, true)
	if err != nil {
		t.Fatalf("create gateway db: %v", err)
	}
	if err := db.AutoMigrate(); err != nil {
		t.Fatalf("migrate gateway db: %v", err)
	}
	client := gatewayclient.New(t.Context(), db, nil, nil, nil, nil, nil, 10*time.Millisecond, 10, 90, 90, 90, true)
	t.Cleanup(func() { _ = client.Close() })
	return client
}

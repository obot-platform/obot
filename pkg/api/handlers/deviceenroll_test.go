package handlers

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	"k8s.io/apiserver/pkg/authentication/user"
)

type deviceLimitProviderFunc func(context.Context) (gatewayclient.DeviceLimit, error)

func (f deviceLimitProviderFunc) DeviceLimit(ctx context.Context) (gatewayclient.DeviceLimit, error) {
	return f(ctx)
}

func TestDeviceEnrollPreservesDeviceLimitForbiddenError(t *testing.T) {
	gatewayClient := newEnforcementTestGatewayClient(t)
	limit := gatewayclient.DeviceLimit{Maximum: 1}

	firstPublicKey := deviceEnrollTestPublicKey(t)
	if _, err := gatewayClient.EnrollDevice(t.Context(), gatewayclient.DeviceEnrollment{
		DeviceID:           "device-1",
		MDMConfigurationID: 1,
		PublicKey:          firstPublicKey,
	}, limit); err != nil {
		t.Fatalf("enroll first device: %v", err)
	}

	body, err := json.Marshal(types.DeviceEnrollRequest{
		DeviceID:  "device-2",
		PublicKey: deviceEnrollTestPublicKey(t),
	})
	if err != nil {
		t.Fatalf("marshal enrollment request: %v", err)
	}

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/mdm/enroll", bytes.NewReader(body))
	apiContext := api.Context{
		ResponseWriter: recorder,
		Request:        req,
		GatewayClient:  gatewayClient,
		User: &user.DefaultInfo{
			Extra: map[string][]string{
				"mdm_configuration_id": {"1"},
			},
		},
	}
	handler := NewDeviceEnrollHandler(deviceLimitProviderFunc(func(context.Context) (gatewayclient.DeviceLimit, error) {
		return limit, nil
	}))

	err = handler.Enroll(apiContext)
	var httpErr *types.ErrHTTP
	if !errors.As(err, &httpErr) {
		t.Fatalf("Enroll() error = %T %v, want *types.ErrHTTP", err, err)
	}
	if httpErr.Code != http.StatusForbidden {
		t.Fatalf("Enroll() HTTP code = %d, want %d", httpErr.Code, http.StatusForbidden)
	}
	if got, err := gatewayClient.DeviceCount(t.Context()); err != nil {
		t.Fatalf("count devices: %v", err)
	} else if got != limit.Maximum {
		t.Fatalf("device count = %d, want %d", got, limit.Maximum)
	}
}

func deviceEnrollTestPublicKey(t *testing.T) []byte {
	t.Helper()
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate Ed25519 key: %v", err)
	}
	der, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	return der
}

func TestDeviceEnrollRejectsInvalidDeviceLimit(t *testing.T) {
	gatewayClient := newEnforcementTestGatewayClient(t)
	body, err := json.Marshal(types.DeviceEnrollRequest{
		DeviceID:  "device-1",
		PublicKey: deviceEnrollTestPublicKey(t),
	})
	if err != nil {
		t.Fatalf("marshal enrollment request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/mdm/enroll", bytes.NewReader(body))
	apiContext := api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        req,
		GatewayClient:  gatewayClient,
		User: &user.DefaultInfo{
			Extra: map[string][]string{
				"mdm_configuration_id": {"1"},
			},
		},
	}
	handler := NewDeviceEnrollHandler(deviceLimitProviderFunc(func(context.Context) (gatewayclient.DeviceLimit, error) {
		return gatewayclient.DeviceLimit{}, nil
	}))

	err = handler.Enroll(apiContext)
	if err == nil || err.Error() != fmt.Sprintf("invalid device limit %d", 0) {
		t.Fatalf("Enroll() error = %v, want invalid device limit", err)
	}
	if got, err := gatewayClient.DeviceCount(t.Context()); err != nil {
		t.Fatalf("count devices: %v", err)
	} else if got != 0 {
		t.Fatalf("device count = %d, want 0", got)
	}
}

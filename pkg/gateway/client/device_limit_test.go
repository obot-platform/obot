package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"

	apitypes "github.com/obot-platform/obot/apiclient/types"
)

func enrollDeviceLimitTestDevice(ctx context.Context, c *Client, deviceID string, deviceLimit DeviceLimit) error {
	_, err := c.EnrollDevice(ctx, DeviceEnrollment{
		DeviceID:           deviceID,
		MDMConfigurationID: 1,
		PublicKey:          []byte("key-" + deviceID),
		Hostname:           "host-" + deviceID,
	}, deviceLimit)
	return err
}

func requireDeviceLimitForbiddenError(t *testing.T, err error) {
	t.Helper()

	var httpErr *apitypes.ErrHTTP
	if !errors.As(err, &httpErr) {
		t.Fatalf("error = %T %v, want *types.ErrHTTP", err, err)
	}
	if httpErr.Code != http.StatusForbidden {
		t.Fatalf("HTTP status = %d, want %d", httpErr.Code, http.StatusForbidden)
	}
}

func TestDefaultDeviceLimit(t *testing.T) {
	if DefaultDeviceLimit != 100 {
		t.Fatalf("DefaultDeviceLimit = %d, want 100", DefaultDeviceLimit)
	}
}

func TestEnrollDeviceEnforcesDeviceLimit(t *testing.T) {
	const maximum = 2
	c := newTestClient(t)
	deviceLimit := DeviceLimit{Maximum: maximum}

	for i := 1; i <= maximum; i++ {
		if err := enrollDeviceLimitTestDevice(t.Context(), c, fmt.Sprintf("device-%d", i), deviceLimit); err != nil {
			t.Fatalf("enrolling device %d: %v", i, err)
		}
	}

	err := enrollDeviceLimitTestDevice(t.Context(), c, "device-3", deviceLimit)
	requireDeviceLimitForbiddenError(t, err)

	count, err := c.DeviceCount(t.Context())
	if err != nil {
		t.Fatalf("counting devices: %v", err)
	}
	if count != maximum {
		t.Fatalf("device count = %d, want %d", count, maximum)
	}
}

func TestEnrollDeviceAllowsExistingDeviceWhenOverLimit(t *testing.T) {
	c := newTestClient(t)
	unlimited := DeviceLimit{Unlimited: true}

	for i := 1; i <= 2; i++ {
		if err := enrollDeviceLimitTestDevice(t.Context(), c, fmt.Sprintf("device-%d", i), unlimited); err != nil {
			t.Fatalf("enrolling device %d: %v", i, err)
		}
	}

	device, err := c.EnrollDevice(t.Context(), DeviceEnrollment{
		DeviceID:           "device-1",
		MDMConfigurationID: 2,
		PublicKey:          []byte("key-device-1"),
		Hostname:           "updated-host",
		OS:                 "darwin",
		OSVersion:          "42",
	}, DeviceLimit{Maximum: 1})
	if err != nil {
		t.Fatalf("re-enrolling existing device while over limit: %v", err)
	}
	if device.MDMConfigurationID != 2 || device.Hostname != "updated-host" || device.OS != "darwin" || device.OSVersion != "42" {
		t.Fatalf("re-enrolled device was not updated: %+v", device)
	}

	_, err = c.EnrollDevice(t.Context(), DeviceEnrollment{
		DeviceID:           "device-1",
		MDMConfigurationID: 2,
		PublicKey:          []byte("different-key"),
	}, DeviceLimit{Maximum: 1})
	if err == nil {
		t.Fatal("re-enrolling existing device with a different key succeeded")
	}
	var httpErr *apitypes.ErrHTTP
	if errors.As(err, &httpErr) {
		t.Fatalf("different-key error = HTTP %d, want identity-key error", httpErr.Code)
	}

	err = enrollDeviceLimitTestDevice(t.Context(), c, "device-3", DeviceLimit{Maximum: 1})
	requireDeviceLimitForbiddenError(t, err)

	count, err := c.DeviceCount(t.Context())
	if err != nil {
		t.Fatalf("counting devices: %v", err)
	}
	if count != 2 {
		t.Fatalf("device count = %d, want 2", count)
	}
}

func TestEnrollDeviceAllowsUnlimitedDevices(t *testing.T) {
	c := newTestClient(t)
	deviceLimit := DeviceLimit{Maximum: 1, Unlimited: true}

	for i := 1; i <= 3; i++ {
		if err := enrollDeviceLimitTestDevice(t.Context(), c, fmt.Sprintf("device-%d", i), deviceLimit); err != nil {
			t.Fatalf("enrolling unlimited device %d: %v", i, err)
		}
	}

	count, err := c.DeviceCount(t.Context())
	if err != nil {
		t.Fatalf("counting devices: %v", err)
	}
	if count != 3 {
		t.Fatalf("device count = %d, want 3", count)
	}
}

func TestDeviceCountIsGlobal(t *testing.T) {
	c := newTestClient(t)
	deviceLimit := DeviceLimit{Unlimited: true}

	for i, configurationID := range []uint{1, 2, 2} {
		_, err := c.EnrollDevice(t.Context(), DeviceEnrollment{
			DeviceID:           fmt.Sprintf("device-%d", i),
			MDMConfigurationID: configurationID,
			PublicKey:          fmt.Appendf(nil, "key-%d", i),
		}, deviceLimit)
		if err != nil {
			t.Fatalf("enrolling device %d: %v", i, err)
		}
	}

	count, err := c.DeviceCount(t.Context())
	if err != nil {
		t.Fatalf("counting devices: %v", err)
	}
	if count != 3 {
		t.Fatalf("device count = %d, want 3", count)
	}
}

func TestEnrollDeviceEnforcesDeviceLimitConcurrently(t *testing.T) {
	const (
		maximum  = 2
		attempts = 8
	)
	c := newTestClient(t)
	deviceLimit := DeviceLimit{Maximum: maximum}

	start := make(chan struct{})
	errorsByAttempt := make(chan error, attempts)
	var wg sync.WaitGroup
	for i := range attempts {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			errorsByAttempt <- enrollDeviceLimitTestDevice(t.Context(), c, fmt.Sprintf("concurrent-device-%d", i), deviceLimit)
		}(i)
	}
	close(start)
	wg.Wait()
	close(errorsByAttempt)

	var succeeded, rejected int
	for err := range errorsByAttempt {
		if err == nil {
			succeeded++
			continue
		}

		var httpErr *apitypes.ErrHTTP
		if errors.As(err, &httpErr) && httpErr.Code == http.StatusForbidden {
			rejected++
			continue
		}
		t.Fatalf("concurrent enrollment returned unexpected error: %v", err)
	}

	if succeeded != maximum {
		t.Fatalf("successful concurrent enrollments = %d, want %d", succeeded, maximum)
	}
	if rejected != attempts-maximum {
		t.Fatalf("rejected concurrent enrollments = %d, want %d", rejected, attempts-maximum)
	}

	count, err := c.DeviceCount(t.Context())
	if err != nil {
		t.Fatalf("counting devices: %v", err)
	}
	if count != maximum {
		t.Fatalf("device count = %d, want %d", count, maximum)
	}
}

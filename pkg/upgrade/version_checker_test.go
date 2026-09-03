package upgrade

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	clienttypes "github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/license"
)

type checkerPropertyClient struct {
	lock  sync.Mutex
	value string
	calls int
	err   error
}

type checkerLicenseProvider struct {
	entitlements []string
	err          error
}

func (c *checkerPropertyClient) GetOrCreateProperty(_ context.Context, key, value string) (gatewaytypes.Property, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.calls++
	if c.err != nil {
		return gatewaytypes.Property{}, c.err
	}
	if c.value == "" {
		c.value = value
	}
	return gatewaytypes.Property{Key: key, Value: c.value}, nil
}

func (c *checkerPropertyClient) callCount() int {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.calls
}

func (p checkerLicenseProvider) Entitlements(context.Context) ([]string, error) {
	return p.entitlements, p.err
}

func testVersionChecker(licenseProvider entitlementProvider, serverURL string) *VersionChecker {
	return &VersionChecker{
		licenseProvider:  licenseProvider,
		httpClient:       http.DefaultClient,
		engine:           "docker",
		currentVersion:   "v1.2.3",
		upgradeServerURL: serverURL,
		checkInterval:    10 * time.Millisecond,
		requestTimeout:   upgradeCheckTimeout,
		done:             make(chan struct{}),
	}
}

func waitForRequest(t *testing.T, requests <-chan *http.Request) *http.Request {
	t.Helper()
	select {
	case request := <-requests:
		return request
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for upgrade request")
		return nil
	}
}

func waitForDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for checker to stop")
	}
}

func waitForStatus(t *testing.T, checker *VersionChecker, expected Status) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if checker.Status() == expected {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("Status() = %#v, want %#v", checker.Status(), expected)
}

func TestNormalizeVersion(t *testing.T) {
	for _, test := range []struct {
		value string
		want  string
	}{
		{
			value: "v1.2.3",
			want:  "v1.2.3",
		},
		{
			value: "v1.2.3+build.4",
			want:  "v1.2.3",
		},
		{
			value: "v1.2.3-rc.1",
			want:  "v1.2.3",
		},
		{
			value: "v1.2.3-rc.1+build.4",
			want:  "v1.2.3",
		},
	} {
		t.Run(test.value, func(t *testing.T) {
			if got := normalizeVersion(test.value); got != test.want {
				t.Fatalf("normalizeVersion(%q) = %q, want %q", test.value, got, test.want)
			}
		})
	}
}

func TestVersionCheckerDoesNotStartWhenDisabledOrDevelopment(t *testing.T) {
	for _, test := range []struct {
		name     string
		disabled bool
		version  string
	}{
		{
			name:     "disabled",
			disabled: true,
			version:  "v1.2.3",
		},
		{
			name:    "development",
			version: "v0.0.0-dev",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			propertyClient := &checkerPropertyClient{}
			checker := &VersionChecker{currentVersion: test.version, done: make(chan struct{})}
			if err := checker.start(t.Context(), propertyClient, test.disabled, false); err != nil {
				t.Fatalf("start() error = %v", err)
			}
			if calls := propertyClient.callCount(); calls != 0 {
				t.Fatalf("installation property calls = %d, want 0", calls)
			}
			waitForDone(t, checker.done)
			if got := checker.Status(); got != (Status{}) {
				t.Fatalf("Status() = %#v", got)
			}
		})
	}
}

func TestVersionCheckerInstallationIDFailureIsSynchronous(t *testing.T) {
	wantErr := errors.New("database unavailable")
	checker := &VersionChecker{currentVersion: "v1.2.3", done: make(chan struct{})}
	err := checker.start(t.Context(), &checkerPropertyClient{err: wantErr}, false, false)
	if !errors.Is(err, wantErr) {
		t.Fatalf("start() error = %v, want %v", err, wantErr)
	}
}

func TestVersionCheckerChecksImmediatelyAndOnSchedule(t *testing.T) {
	requests := make(chan *http.Request, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requests <- request.Clone(request.Context())
		_ = json.NewEncoder(w).Encode(upgradeCheckResponse{
			UpgradeAvailable: true,
			LatestVersion:    "v1.3.0",
			CurrentVersion:   "v1.2.3",
		})
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(t.Context())
	propertyClient := &checkerPropertyClient{value: "installation-1"}
	checker := testVersionChecker(checkerLicenseProvider{entitlements: []string{license.EnterpriseEntitlement}}, server.URL+"/check-upgrade")
	checker.currentVersion = "v1.2.3-rc.1+build.4"
	if err := checker.start(ctx, propertyClient, false, true); err != nil {
		t.Fatalf("start() error = %v", err)
	}

	request := waitForRequest(t, requests)
	if request.Method != http.MethodGet {
		t.Fatalf("method = %q, want GET", request.Method)
	}
	if request.URL.Path != "/check-upgrade" {
		t.Fatalf("path = %q", request.URL.Path)
	}
	query := request.URL.Query()
	for key, want := range map[string]string{
		"uid":             "installation-1",
		"engine":          "docker",
		"distribution":    string(clienttypes.ProductTelemetryDistributionEnterprise),
		"current-version": "v1.2.3",
	} {
		if got := query.Get(key); got != want {
			t.Errorf("query %q = %q, want %q", key, got, want)
		}
	}
	waitForStatus(t, checker, Status{UpgradeAvailable: true, LatestVersion: "v1.3.0"})

	waitForRequest(t, requests)

	cancel()
	waitForDone(t, checker.done)
}

func TestVersionCheckerDevelopmentVersionCanBeForced(t *testing.T) {
	requests := make(chan *http.Request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requests <- request.Clone(request.Context())
		_, _ = w.Write([]byte(`{"upgradeAvailable":false,"latestVersion":""}`))
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(t.Context())
	checker := testVersionChecker(checkerLicenseProvider{}, server.URL)
	checker.currentVersion = "v0.0.0-dev"
	if err := checker.start(ctx, &checkerPropertyClient{value: "installation"}, false, true); err != nil {
		t.Fatalf("start() error = %v", err)
	}
	waitForRequest(t, requests)
	cancel()
	waitForDone(t, checker.done)
}

func TestVersionCheckerDistribution(t *testing.T) {
	for _, test := range []struct {
		name     string
		provider checkerLicenseProvider
		want     clienttypes.ProductTelemetryDistribution
	}{
		{
			name:     "cloud",
			provider: checkerLicenseProvider{entitlements: []string{license.CloudEntitlement}},
			want:     clienttypes.ProductTelemetryDistributionCloud,
		},
		{
			name:     "enterprise",
			provider: checkerLicenseProvider{entitlements: []string{license.EnterpriseEntitlement}},
			want:     clienttypes.ProductTelemetryDistributionEnterprise,
		},
		{
			name:     "registered",
			provider: checkerLicenseProvider{entitlements: []string{license.CommunityEntitlement}},
			want:     clienttypes.ProductTelemetryDistributionRegistered,
		},
		{
			name: "enterprise takes precedence",
			provider: checkerLicenseProvider{entitlements: []string{
				license.CommunityEntitlement,
				license.EnterpriseEntitlement,
			}},
			want: clienttypes.ProductTelemetryDistributionEnterprise,
		},
		{
			name: "cloud takes precedence",
			provider: checkerLicenseProvider{entitlements: []string{
				license.CommunityEntitlement,
				license.EnterpriseEntitlement,
				license.CloudEntitlement,
			}},
			want: clienttypes.ProductTelemetryDistributionCloud,
		},
		{
			name:     "unregistered",
			provider: checkerLicenseProvider{},
			want:     clienttypes.ProductTelemetryDistributionUnregistered,
		},
		{
			name:     "license error",
			provider: checkerLicenseProvider{err: errors.New("refresh failed")},
			want:     clienttypes.ProductTelemetryDistributionUnregistered,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			requests := make(chan *http.Request, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				requests <- request.Clone(request.Context())
				_, _ = w.Write([]byte(`{"upgradeAvailable":false,"latestVersion":""}`))
			}))
			defer server.Close()

			ctx, cancel := context.WithCancel(t.Context())
			checker := testVersionChecker(test.provider, server.URL)
			if err := checker.start(ctx, &checkerPropertyClient{value: "installation"}, false, false); err != nil {
				t.Fatalf("start() error = %v", err)
			}
			request := waitForRequest(t, requests)
			if got := request.URL.Query().Get("distribution"); got != string(test.want) {
				t.Fatalf("distribution = %q, want %q", got, test.want)
			}
			cancel()
			waitForDone(t, checker.done)
		})
	}
}

func TestVersionCheckerResponseFailuresDoNotChangeStatusOrStopSchedule(t *testing.T) {
	for _, test := range []struct {
		name       string
		statusCode int
		body       string
	}{
		{
			name:       "non-OK",
			statusCode: http.StatusBadGateway,
			body:       "upgrade server unavailable",
		},
		{
			name:       "malformed JSON",
			statusCode: http.StatusOK,
			body:       `{"upgradeAvailable":`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			requests := make(chan *http.Request, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				requests <- request.Clone(request.Context())
				w.WriteHeader(test.statusCode)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()

			ctx, cancel := context.WithCancel(t.Context())
			checker := testVersionChecker(checkerLicenseProvider{}, server.URL)
			if err := checker.start(ctx, &checkerPropertyClient{value: "installation"}, false, false); err != nil {
				t.Fatalf("start() error = %v", err)
			}
			waitForRequest(t, requests)
			waitForRequest(t, requests)
			if got := checker.Status(); got != (Status{}) {
				t.Fatalf("Status() = %#v, want zero status", got)
			}
			select {
			case <-checker.done:
				t.Fatal("checker stopped after request failure")
			default:
			}
			cancel()
			waitForDone(t, checker.done)
		})
	}
}

func TestVersionCheckerRequestTimeoutDoesNotStopSchedule(t *testing.T) {
	requests := make(chan struct{}, 2)
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		requests <- struct{}{}
		<-request.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(t.Context())
	checker := testVersionChecker(checkerLicenseProvider{}, server.URL)
	checker.requestTimeout = 20 * time.Millisecond
	if err := checker.start(ctx, &checkerPropertyClient{value: "installation"}, false, false); err != nil {
		t.Fatalf("start() error = %v", err)
	}
	select {
	case <-requests:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for request")
	}
	select {
	case <-requests:
	case <-time.After(2 * time.Second):
		t.Fatal("checker did not run again after request timeout")
	}
	cancel()
	waitForDone(t, checker.done)
}

func TestVersionCheckerStatusSupportsConcurrentReaders(_ *testing.T) {
	checker := &VersionChecker{}
	var readers sync.WaitGroup
	for range 20 {
		readers.Go(func() {
			for range 100 {
				_ = checker.Status()
			}
		})
	}
	for range 100 {
		checker.statusLock.Lock()
		checker.status = Status{UpgradeAvailable: true, LatestVersion: "v1.3.0"}
		checker.statusLock.Unlock()
	}
	readers.Wait()
}

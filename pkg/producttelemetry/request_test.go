package producttelemetry

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/version"
)

type requestPropertyClient struct {
	lock  sync.Mutex
	value string
	calls int
	err   error
}

func (c *requestPropertyClient) GetOrCreateProperty(_ context.Context, _ string, value string) (gatewaytypes.Property, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.calls++
	if c.err != nil {
		return gatewaytypes.Property{}, c.err
	}
	if c.value == "" {
		c.value = value
	}
	return gatewaytypes.Property{Value: c.value}, nil
}

func TestBuildRequestPopulatesInstallationIDAndVersion(t *testing.T) {
	propertyClient := &requestPropertyClient{}

	first, err := buildRequest(t.Context(), propertyClient)
	if err != nil {
		t.Fatalf("first buildRequest() error = %v", err)
	}
	second, err := buildRequest(t.Context(), propertyClient)
	if err != nil {
		t.Fatalf("second buildRequest() error = %v", err)
	}
	if first.InstallationID == "" || second.InstallationID != first.InstallationID {
		t.Fatalf("installation IDs = %q, %q, want one stable non-empty value", first.InstallationID, second.InstallationID)
	}
	if propertyClient.calls != 2 {
		t.Fatalf("property calls = %d, want 2", propertyClient.calls)
	}
	if first.CurrentVersion != version.Get().String() || second.CurrentVersion != first.CurrentVersion {
		t.Fatalf("versions = %q, %q, want %q", first.CurrentVersion, second.CurrentVersion, version.Get().String())
	}

	body, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	want := `{"installationID":"` + first.InstallationID + `","licenseMachineID":"","reportedAt":"0001-01-01T00:00:00Z","distribution":"","engine":"","currentVersion":"` + version.Get().String() + `"}`
	if string(body) != want {
		t.Fatalf("report body = %s, want %s", body, want)
	}
}

func TestBuildRequestInstallationIDFailure(t *testing.T) {
	_, err := buildRequest(t.Context(), &requestPropertyClient{err: errors.New("database unavailable")})
	if err == nil || !strings.Contains(err.Error(), "get installation ID") {
		t.Fatalf("buildRequest() error = %v, want collection context", err)
	}
}

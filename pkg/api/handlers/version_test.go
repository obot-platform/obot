package handlers

import (
	"testing"

	"github.com/obot-platform/obot/pkg/upgrade"
)

type fakeUpgradeStatusReader struct {
	status upgrade.Status
}

func (f fakeUpgradeStatusReader) Status() upgrade.Status {
	return f.status
}

func TestVersionFeatureValuesIncludesHostedAgentsState(t *testing.T) {
	disabled := (&VersionHandler{}).featureValues()
	if enabled := disabled["hostedAgentsEnabled"]; enabled {
		t.Fatal("expected Hosted Agents to be disabled")
	}

	handler := &VersionHandler{}
	handler.HostedAgentsEnabled = true
	enabled := handler.featureValues()
	if hostedAgentsEnabled := enabled["hostedAgentsEnabled"]; !hostedAgentsEnabled {
		t.Fatal("expected Hosted Agents to be enabled")
	}
}

func TestVersionHandlerReadsUpgradeStatus(t *testing.T) {
	want := upgrade.Status{UpgradeAvailable: true, LatestVersion: "v1.2.3"}
	handler := &VersionHandler{}
	handler.UpgradeStatusReader = fakeUpgradeStatusReader{status: want}
	if got := handler.upgradeStatus(); got != want {
		t.Fatalf("upgradeStatus() = %#v, want %#v", got, want)
	}

	handler.UpgradeStatusReader = nil
	if got := handler.upgradeStatus(); got != (upgrade.Status{}) {
		t.Fatalf("upgradeStatus() without reader = %#v, want zero status", got)
	}
}

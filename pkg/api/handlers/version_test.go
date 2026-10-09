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

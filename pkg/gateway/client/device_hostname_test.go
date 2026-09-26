package client

import (
	"context"
	"testing"
	"time"

	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
)

func enrollHostnameTestDevice(t *testing.T, ctx context.Context, c *Client, deviceID, hostname string) {
	t.Helper()
	if _, err := c.EnrollDevice(ctx, DeviceEnrollment{
		DeviceID:           deviceID,
		MDMConfigurationID: 1,
		PublicKey:          []byte("key-" + deviceID),
		Hostname:           hostname,
	}, DeviceLimit{Maximum: 100, Unlimited: true}); err != nil {
		t.Fatalf("enroll %s: %v", deviceID, err)
	}
}

func TestDeviceHostnamesByDeviceID(t *testing.T) {
	ctx := context.Background()
	c := newTestClient(t)

	enrollHostnameTestDevice(t, ctx, c, "dev-a", "alice-macbook")
	enrollHostnameTestDevice(t, ctx, c, "dev-b", "bob-thinkpad")

	t.Run("resolves only the requested devices", func(t *testing.T) {
		got, err := c.DeviceHostnamesByDeviceID(ctx, []string{"dev-a"})
		if err != nil {
			t.Fatalf("DeviceHostnamesByDeviceID: %v", err)
		}
		if len(got) != 1 || got["dev-a"] != "alice-macbook" {
			t.Fatalf("got %v, want only dev-a", got)
		}
	})

	t.Run("resolves several at once", func(t *testing.T) {
		got, err := c.DeviceHostnamesByDeviceID(ctx, []string{"dev-a", "dev-b", "dev-missing"})
		if err != nil {
			t.Fatalf("DeviceHostnamesByDeviceID: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("got %v, want 2 entries (dev-missing has no row)", got)
		}
		if got["dev-a"] != "alice-macbook" || got["dev-b"] != "bob-thinkpad" {
			t.Fatalf("got %v, want the enrolled hostnames", got)
		}
	})

	t.Run("empty input short-circuits", func(t *testing.T) {
		got, err := c.DeviceHostnamesByDeviceID(ctx, nil)
		if err != nil {
			t.Fatalf("DeviceHostnamesByDeviceID: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("got %v, want empty", got)
		}
	})
}

func TestDeviceIDsMatchingHostnames(t *testing.T) {
	ctx := context.Background()
	c := newTestClient(t)

	enrollHostnameTestDevice(t, ctx, c, "dev-a", "alice-macbook")
	enrollHostnameTestDevice(t, ctx, c, "dev-b", "bob-thinkpad")
	// A literal % in a hostname: it must be matched literally, not treated as a wildcard.
	enrollHostnameTestDevice(t, ctx, c, "dev-pct", "build%agent")

	t.Run("substring match is case-insensitive", func(t *testing.T) {
		got, err := c.DeviceIDsMatchingHostnames(ctx, []string{"MACBOOK"})
		if err != nil {
			t.Fatalf("DeviceIDsMatchingHostnames: %v", err)
		}
		if len(got) != 1 || got[0] != "dev-a" {
			t.Fatalf("got %v, want [dev-a]", got)
		}
	})

	t.Run("no match returns empty", func(t *testing.T) {
		got, err := c.DeviceIDsMatchingHostnames(ctx, []string{"nonexistent"})
		if err != nil {
			t.Fatalf("DeviceIDsMatchingHostnames: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("got %v, want empty", got)
		}
	})

	t.Run("multiple patterns union without duplicates", func(t *testing.T) {
		got, err := c.DeviceIDsMatchingHostnames(ctx, []string{"alice", "macbook"})
		if err != nil {
			t.Fatalf("DeviceIDsMatchingHostnames: %v", err)
		}
		if len(got) != 1 || got[0] != "dev-a" {
			t.Fatalf("got %v, want [dev-a] exactly once", got)
		}
	})

	t.Run("wildcards are escaped, not honoured", func(t *testing.T) {
		// "%" alone would match every hostname if it leaked into the LIKE pattern.
		got, err := c.DeviceIDsMatchingHostnames(ctx, []string{"%"})
		if err != nil {
			t.Fatalf("DeviceIDsMatchingHostnames: %v", err)
		}
		if len(got) != 1 || got[0] != "dev-pct" {
			t.Fatalf("got %v, want only the literal-percent host [dev-pct]", got)
		}
	})
}

func TestRefreshDeviceFromScan(t *testing.T) {
	ctx := context.Background()
	c := newTestClient(t)

	enrollHostnameTestDevice(t, ctx, c, "dev-a", "old-name")
	seen := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)

	t.Run("updates hostname and last seen", func(t *testing.T) {
		if err := c.RefreshDeviceFromScan(ctx, "dev-a", "new-name", seen); err != nil {
			t.Fatalf("RefreshDeviceFromScan: %v", err)
		}
		device, err := c.GetDeviceByDeviceID(ctx, "dev-a")
		if err != nil {
			t.Fatalf("GetDeviceByDeviceID: %v", err)
		}
		if device.Hostname != "new-name" {
			t.Fatalf("Hostname = %q, want %q", device.Hostname, "new-name")
		}
		if device.LastSeenAt == nil || !device.LastSeenAt.UTC().Equal(seen) {
			t.Fatalf("LastSeenAt = %v, want %v", device.LastSeenAt, seen)
		}
	})

	t.Run("empty hostname only refreshes last seen", func(t *testing.T) {
		later := seen.Add(time.Hour)
		if err := c.RefreshDeviceFromScan(ctx, "dev-a", "", later); err != nil {
			t.Fatalf("RefreshDeviceFromScan: %v", err)
		}
		device, err := c.GetDeviceByDeviceID(ctx, "dev-a")
		if err != nil {
			t.Fatalf("GetDeviceByDeviceID: %v", err)
		}
		if device.Hostname != "new-name" {
			t.Fatalf("Hostname = %q, want it preserved as %q", device.Hostname, "new-name")
		}
		if device.LastSeenAt == nil || !device.LastSeenAt.UTC().Equal(later) {
			t.Fatalf("LastSeenAt = %v, want %v", device.LastSeenAt, later)
		}
	})

	t.Run("unenrolled device is not created", func(t *testing.T) {
		if err := c.RefreshDeviceFromScan(ctx, "dev-never-enrolled", "ghost", seen); err != nil {
			t.Fatalf("RefreshDeviceFromScan: %v", err)
		}
		if _, err := c.GetDeviceByDeviceID(ctx, "dev-never-enrolled"); err == nil {
			t.Fatal("expected no device row for an unenrolled ID, but one was found")
		}
	})

	t.Run("empty device ID is a no-op", func(t *testing.T) {
		if err := c.RefreshDeviceFromScan(ctx, "", "whatever", seen); err != nil {
			t.Fatalf("RefreshDeviceFromScan: %v", err)
		}
	})
}

func TestInsertDeviceScanRefreshesRegistry(t *testing.T) {
	ctx := context.Background()
	c := newTestClient(t)

	enrollHostnameTestDevice(t, ctx, c, "dev-a", "enrolled-name")

	scan := &gatewaytypes.DeviceScan{
		DeviceID:  "dev-a",
		Hostname:  "scanned-name",
		ScannedAt: time.Date(2026, 9, 26, 13, 0, 0, 0, time.UTC),
	}
	if err := c.InsertDeviceScan(ctx, scan); err != nil {
		t.Fatalf("InsertDeviceScan: %v", err)
	}

	device, err := c.GetDeviceByDeviceID(ctx, "dev-a")
	if err != nil {
		t.Fatalf("GetDeviceByDeviceID: %v", err)
	}
	if device.Hostname != "scanned-name" {
		t.Fatalf("Hostname = %q, want the scan to have refreshed it to %q", device.Hostname, "scanned-name")
	}
}

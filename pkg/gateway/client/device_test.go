package client

import (
	"errors"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

func TestDeleteDeviceFreesSeat(t *testing.T) {
	c := newTestClient(t)
	limit := DeviceLimit{Maximum: 1}

	if err := enrollDeviceLimitTestDevice(t.Context(), c, "device-1", limit); err != nil {
		t.Fatalf("enrolling device-1: %v", err)
	}
	requireDeviceLimitForbiddenError(t, enrollDeviceLimitTestDevice(t.Context(), c, "device-2", limit))

	if err := c.DeleteDevice(t.Context(), "device-1", false); err != nil {
		t.Fatalf("deleting device-1: %v", err)
	}
	if _, err := c.GetDeviceByDeviceID(t.Context(), "device-1"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("looking up deleted device: error = %v, want %v", err, gorm.ErrRecordNotFound)
	}
	if err := c.DeleteDevice(t.Context(), "device-1", false); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("deleting missing device: error = %v, want %v", err, gorm.ErrRecordNotFound)
	}
	if err := enrollDeviceLimitTestDevice(t.Context(), c, "device-2", limit); err != nil {
		t.Fatalf("enrolling device-2 after freeing a seat: %v", err)
	}
}

func TestDeleteDeviceScans(t *testing.T) {
	for _, tt := range []struct {
		name          string
		deleteScans   bool
		wantScanCount int64
	}{
		{
			name:          "keeps scans by default",
			deleteScans:   false,
			wantScanCount: 2,
		},
		{
			name:          "deletes scans when asked",
			deleteScans:   true,
			wantScanCount: 0,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t)

			for _, deviceID := range []string{"device-1", "device-2"} {
				if err := enrollDeviceLimitTestDevice(t.Context(), c, deviceID, DeviceLimit{Unlimited: true}); err != nil {
					t.Fatalf("enrolling %s: %v", deviceID, err)
				}
			}
			for _, deviceID := range []string{"device-1", "device-1", "device-2"} {
				scan := types.DeviceScan{
					DeviceID:  deviceID,
					ScannedAt: time.Now(),
					MCPServers: []types.DeviceScanMCPServer{
						{
							Name: "github",
						},
					},
				}
				if err := c.InsertDeviceScan(t.Context(), &scan); err != nil {
					t.Fatalf("inserting scan for %s: %v", deviceID, err)
				}
			}

			if err := c.DeleteDevice(t.Context(), "device-1", tt.deleteScans); err != nil {
				t.Fatalf("deleting device-1: %v", err)
			}

			var scanCount, observationCount int64
			if err := c.db.WithContext(t.Context()).Model(&types.DeviceScan{}).Where("device_id = ?", "device-1").Count(&scanCount).Error; err != nil {
				t.Fatalf("counting device-1 scans: %v", err)
			}
			if scanCount != tt.wantScanCount {
				t.Fatalf("device-1 scans = %d, want %d", scanCount, tt.wantScanCount)
			}
			// device-2's scan and its observation are left alone either way
			if err := c.db.WithContext(t.Context()).Model(&types.DeviceScanMCPServer{}).Count(&observationCount).Error; err != nil {
				t.Fatalf("counting scan observations: %v", err)
			}
			if want := tt.wantScanCount + 1; observationCount != want {
				t.Fatalf("scan observations = %d, want %d", observationCount, want)
			}
		})
	}
}

func TestListAllDevicesIncludesEveryConfigurationAndLatestScan(t *testing.T) {
	c := newTestClient(t)
	unlimited := DeviceLimit{Unlimited: true}

	for _, enrollment := range []DeviceEnrollment{
		{
			DeviceID:           "device-1",
			MDMConfigurationID: 1,
			PublicKey:          []byte("key-device-1"),
		},
		{
			DeviceID:           "device-2",
			MDMConfigurationID: 2,
			PublicKey:          []byte("key-device-2"),
		},
	} {
		if _, err := c.EnrollDevice(t.Context(), enrollment, unlimited); err != nil {
			t.Fatalf("enrolling %s: %v", enrollment.DeviceID, err)
		}
	}

	scannedAt := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	for _, scan := range []types.DeviceScan{
		{
			DeviceID:  "device-1",
			ScannedAt: scannedAt.Add(-time.Hour),
			Skills: []types.DeviceScanSkill{
				{
					Name: "stale-skill",
				},
			},
		},
		{
			DeviceID:  "device-1",
			Hostname:  "host-1",
			Username:  "alice",
			OS:        "darwin",
			Arch:      "arm64",
			ScannedAt: scannedAt,
			MCPServers: []types.DeviceScanMCPServer{
				{
					Name: "github",
				},
				{
					Name: "filesystem",
				},
			},
			Clients: []types.DeviceScanClient{
				{
					Name: "cursor",
				},
			},
		},
		{
			DeviceID:  "unenrolled",
			ScannedAt: scannedAt,
		},
	} {
		if err := c.InsertDeviceScan(t.Context(), &scan); err != nil {
			t.Fatalf("inserting scan: %v", err)
		}
	}

	devices, err := c.ListAllDevices(t.Context())
	if err != nil {
		t.Fatalf("listing devices: %v", err)
	}
	byID := make(map[string]types.Device, len(devices))
	for _, device := range devices {
		byID[device.DeviceID] = device
	}
	if len(byID) != 2 {
		t.Fatalf("devices = %v, want device-1 and device-2", byID)
	}
	latest := byID["device-1"].LatestScan
	if latest == nil {
		t.Fatal("device-1 latest scan = nil, want its newest scan")
	}
	if !latest.ScannedAt.Equal(scannedAt) || latest.Hostname != "host-1" || latest.Username != "alice" || latest.Arch != "arm64" {
		t.Fatalf("device-1 latest scan = {scannedAt: %v, hostname: %q, username: %q, arch: %q}, want {scannedAt: %v, hostname: %q, username: %q, arch: %q}",
			latest.ScannedAt, latest.Hostname, latest.Username, latest.Arch, scannedAt, "host-1", "alice", "arm64")
	}
	if latest.MCPServerCount != 2 || latest.SkillCount != 0 || latest.PluginCount != 0 || latest.ClientCount != 1 {
		t.Fatalf("device-1 latest scan counts = {mcpServers: %d, skills: %d, plugins: %d, clients: %d}, want {mcpServers: 2, skills: 0, plugins: 0, clients: 1}",
			latest.MCPServerCount, latest.SkillCount, latest.PluginCount, latest.ClientCount)
	}
	if got := byID["device-2"].LatestScan; got != nil {
		t.Fatalf("device-2 latest scan = %v, want nil", got)
	}
}

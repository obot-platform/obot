package authz

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gtypes "github.com/obot-platform/obot/pkg/gateway/types"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"gorm.io/gorm"
	"k8s.io/apiserver/pkg/authentication/user"
)

// TestCheckDeviceScanOwnership checks who may fetch a scan by ID: a device
// only its own device uploads, a human only scans they submitted.
func TestCheckDeviceScanOwnership(t *testing.T) {
	gatewayClient := newDeviceScanTestGatewayClient(t)
	authorizer := &Authorizer{gatewayClient: gatewayClient}

	insert := func(submittedBy, deviceID string) string {
		t.Helper()
		scan := gtypes.DeviceScan{
			SubmittedBy: submittedBy,
			DeviceID:    deviceID,
		}
		if err := gatewayClient.InsertDeviceScan(t.Context(), &scan); err != nil {
			t.Fatalf("insert device scan: %v", err)
		}
		return strconv.FormatUint(uint64(scan.ID), 10)
	}

	// device-a must be enrolled before its scans: a device owns only scans
	// received during its current enrollment.
	enrollDeviceScanTestDevice(t, gatewayClient, "device-a", "key-1")

	ownScan := insert("", "device-a")
	otherDeviceScan := insert("", "device-b")
	humanSameDeviceIDScan := insert("1", "device-a")

	device := User{
		Info: &user.DefaultInfo{
			Name:   gtypes.DevicePrincipalName("device-a"),
			UID:    gtypes.DevicePrincipalName("device-a"),
			Groups: []string{types.GroupDeviceScans, types.GroupAuthenticated},
			Extra: map[string][]string{
				"device_id": {"device-a"},
			},
		},
	}
	human := User{
		Info: &user.DefaultInfo{
			Name:   "human",
			UID:    "1",
			Groups: []string{types.GroupAuthenticated},
		},
	}
	forgedHuman := User{
		Info: &user.DefaultInfo{
			Name:   "other-human",
			UID:    "2",
			Groups: []string{types.GroupAuthenticated},
			Extra: map[string][]string{
				"device_id": {"device-a"},
			},
		},
	}

	tests := []struct {
		name   string
		user   User
		scanID string
		want   bool
	}{
		{
			name:   "device reads its own device upload",
			user:   device,
			scanID: ownScan,
			want:   true,
		},
		{
			name:   "device cannot read another device's scan",
			user:   device,
			scanID: otherDeviceScan,
			want:   false,
		},
		{
			name:   "device cannot read a human scan carrying its device ID",
			user:   device,
			scanID: humanSameDeviceIDScan,
			want:   false,
		},
		{
			name:   "human reads a scan they submitted",
			user:   human,
			scanID: humanSameDeviceIDScan,
			want:   true,
		},
		{
			name:   "human cannot read a device upload",
			user:   human,
			scanID: ownScan,
			want:   false,
		},
		{
			name:   "human with forged device_id extra is not treated as a device",
			user:   forgedHuman,
			scanID: ownScan,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/devices/scans/"+tt.scanID, nil)

			got, err := authorizer.checkDeviceScan(req, &Resources{DeviceScanID: tt.scanID}, tt.user)
			if err != nil {
				t.Fatalf("checkDeviceScan() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("checkDeviceScan() = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("device without a device row is denied", func(t *testing.T) {
		// device-z was never enrolled (or was deleted after authenticating), so
		// even a device upload carrying its ID must not be granted.
		unenrolledScan := insert("", "device-z")
		unenrolled := User{
			Info: &user.DefaultInfo{
				Name:   gtypes.DevicePrincipalName("device-z"),
				UID:    gtypes.DevicePrincipalName("device-z"),
				Groups: []string{types.GroupDeviceScans, types.GroupAuthenticated},
				Extra: map[string][]string{
					"device_id": {"device-z"},
				},
			},
		}
		req := httptest.NewRequest(http.MethodGet, "/api/devices/scans/"+unenrolledScan, nil)

		got, err := authorizer.checkDeviceScan(req, &Resources{DeviceScanID: unenrolledScan}, unenrolled)
		if err != nil || got {
			t.Fatalf("checkDeviceScan() = %v, %v; want false, nil", got, err)
		}
	})

	t.Run("missing scan keeps the lookup error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/devices/scans/9999", nil)

		got, err := authorizer.checkDeviceScan(req, &Resources{DeviceScanID: "9999"}, device)
		if got || !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("checkDeviceScan() = %v, %v; want false, %v", got, err, gorm.ErrRecordNotFound)
		}
	})
}

// TestCheckDeviceScanEarlierEnrollment checks that re-enrolling a deleted
// device's ID with a new key does not grant the scans kept from the earlier
// enrollment, while scans uploaded since then are still granted.
func TestCheckDeviceScanEarlierEnrollment(t *testing.T) {
	gatewayClient := newDeviceScanTestGatewayClient(t)
	authorizer := &Authorizer{gatewayClient: gatewayClient}

	enrollDeviceScanTestDevice(t, gatewayClient, "device-a", "key-1")
	oldScan := gtypes.DeviceScan{DeviceID: "device-a"}
	if err := gatewayClient.InsertDeviceScan(t.Context(), &oldScan); err != nil {
		t.Fatalf("insert old device scan: %v", err)
	}

	// An admin deletes the device but keeps its scans; the ID is then enrolled
	// again with a different identity key.
	if err := gatewayClient.DeleteDevice(t.Context(), "device-a", false); err != nil {
		t.Fatalf("delete device: %v", err)
	}
	enrollDeviceScanTestDevice(t, gatewayClient, "device-a", "key-2")

	newScan := gtypes.DeviceScan{DeviceID: "device-a"}
	if err := gatewayClient.InsertDeviceScan(t.Context(), &newScan); err != nil {
		t.Fatalf("insert new device scan: %v", err)
	}

	device := User{
		Info: &user.DefaultInfo{
			Name:   gtypes.DevicePrincipalName("device-a"),
			UID:    gtypes.DevicePrincipalName("device-a"),
			Groups: []string{types.GroupDeviceScans, types.GroupAuthenticated},
			Extra: map[string][]string{
				"device_id": {"device-a"},
			},
		},
	}

	tests := []struct {
		name   string
		scanID uint
		want   bool
	}{
		{
			name:   "scan from the earlier enrollment is denied",
			scanID: oldScan.ID,
			want:   false,
		},
		{
			name:   "scan from the current enrollment is granted",
			scanID: newScan.ID,
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scanID := strconv.FormatUint(uint64(tt.scanID), 10)
			req := httptest.NewRequest(http.MethodGet, "/api/devices/scans/"+scanID, nil)

			got, err := authorizer.checkDeviceScan(req, &Resources{DeviceScanID: scanID}, device)
			if err != nil {
				t.Fatalf("checkDeviceScan() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("checkDeviceScan() = %v, want %v", got, tt.want)
			}
		})
	}
}

// enrollDeviceScanTestDevice enrolls deviceID with publicKey as a placeholder
// key. Enrollment only compares key bytes and never parses them, and these
// tests build the device principal directly instead of verifying a JWT, so the
// key doesn't need to be real. A different publicKey models a new identity key.
func enrollDeviceScanTestDevice(t *testing.T, c *gatewayclient.Client, deviceID, publicKey string) {
	t.Helper()

	if _, err := c.EnrollDevice(t.Context(), gatewayclient.DeviceEnrollment{
		DeviceID:           deviceID,
		MDMConfigurationID: 1,
		PublicKey:          []byte(publicKey),
	}, gatewayclient.DeviceLimit{Unlimited: true}); err != nil {
		t.Fatalf("enroll device %q: %v", deviceID, err)
	}
}

// newDeviceScanTestGatewayClient mirrors newEnforcementTestGatewayClient in
// pkg/api/handlers. That helper lives in another package's test files, so this
// package can't import it.
func newDeviceScanTestGatewayClient(t *testing.T) *gatewayclient.Client {
	t.Helper()

	storageServices, err := sservices.New(sservices.Config{DSN: "sqlite://:memory:"})
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
	c := gatewayclient.New(t.Context(), db, nil, nil, nil, nil, nil, 10*time.Millisecond, 10, 90, 90, 90, true)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gtypes "github.com/obot-platform/obot/pkg/gateway/types"
	"k8s.io/apiserver/pkg/authentication/user"
)

// TestDeviceScansListScoping checks which scans each kind of caller can list,
// and that client-supplied filters never widen a non-privileged caller's set.
func TestDeviceScansListScoping(t *testing.T) {
	gatewayClient := newEnforcementTestGatewayClient(t)

	insert := func(submittedBy, deviceID string) uint {
		t.Helper()
		scan := gtypes.DeviceScan{
			SubmittedBy: submittedBy,
			DeviceID:    deviceID,
		}
		if err := gatewayClient.InsertDeviceScan(t.Context(), &scan); err != nil {
			t.Fatalf("insert device scan: %v", err)
		}
		return scan.ID
	}

	// device-a must be enrolled before its scans: a device lists only scans
	// received during its current enrollment.
	enrollDeviceScansTestDevice(t, gatewayClient, "device-a", "key-1")

	// Insertion order matters: the human scan carrying device-a's ID is the
	// newest device-a row, so if the GroupByDevice latest-per-device subquery
	// ignored the device-upload filter, device-a's list would come back empty.
	ownOld := insert("", "device-a")
	ownNew := insert("", "device-a")
	otherDevice := insert("", "device-b")
	otherHuman := insert("2", "device-c")
	humanSameDeviceID := insert("1", "device-a")
	all := []uint{ownOld, ownNew, otherDevice, otherHuman, humanSameDeviceID}

	device := &user.DefaultInfo{
		Name:   gtypes.DevicePrincipalName("device-a"),
		UID:    gtypes.DevicePrincipalName("device-a"),
		Groups: []string{types.GroupDeviceScans, types.GroupAuthenticated},
		Extra: map[string][]string{
			"device_id": {"device-a"},
		},
	}
	human := &user.DefaultInfo{
		Name:   "human",
		UID:    "1",
		Groups: []string{types.GroupAuthenticated},
	}

	tests := []struct {
		name  string
		user  user.Info
		query string
		want  []uint
	}{
		{
			name:  "device sees its latest own scan when grouped",
			user:  device,
			query: "",
			want:  []uint{ownNew},
		},
		{
			name:  "device sees only its own device uploads",
			user:  device,
			query: "?group_by_device=false",
			want:  []uint{ownOld, ownNew},
		},
		{
			name:  "device cannot widen with device_id",
			user:  device,
			query: "?group_by_device=false&device_id=device-b",
			want:  []uint{ownOld, ownNew},
		},
		{
			name:  "device cannot widen with submitted_by",
			user:  device,
			query: "?group_by_device=false&submitted_by=1",
			want:  []uint{ownOld, ownNew},
		},
		{
			name: "device without a device row sees nothing",
			user: &user.DefaultInfo{
				Name:   gtypes.DevicePrincipalName("device-z"),
				UID:    gtypes.DevicePrincipalName("device-z"),
				Groups: []string{types.GroupDeviceScans, types.GroupAuthenticated},
				Extra: map[string][]string{
					"device_id": {"device-z"},
				},
			},
			query: "?group_by_device=false&device_id=device-a",
			want:  nil,
		},
		{
			name:  "human sees only scans they submitted",
			user:  human,
			query: "?group_by_device=false",
			want:  []uint{humanSameDeviceID},
		},
		{
			name:  "human cannot widen with submitted_by",
			user:  human,
			query: "?group_by_device=false&submitted_by=2",
			want:  []uint{humanSameDeviceID},
		},
		{
			name: "human with forged device_id extra is not treated as a device",
			user: &user.DefaultInfo{
				Name:   "human",
				UID:    "1",
				Groups: []string{types.GroupAuthenticated},
				Extra: map[string][]string{
					"device_id": {"device-a"},
				},
			},
			query: "?group_by_device=false",
			want:  []uint{humanSameDeviceID},
		},
		{
			name: "admin sees all scans",
			user: &user.DefaultInfo{
				Name:   "admin",
				UID:    "3",
				Groups: []string{types.GroupAdmin},
			},
			query: "?group_by_device=false",
			want:  all,
		},
		{
			name: "auditor sees all scans",
			user: &user.DefaultInfo{
				Name:   "auditor",
				UID:    "4",
				Groups: []string{types.GroupAuditor},
			},
			query: "?group_by_device=false",
			want:  all,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := listDeviceScanIDs(t, gatewayClient, tt.user, tt.query)

			// Rows inserted in one test can share created_at, so compare as sets.
			want := slices.Sorted(slices.Values(tt.want))
			if !slices.Equal(got, want) {
				t.Fatalf("List() scan IDs = %v, want %v", got, want)
			}
		})
	}
}

// TestDeviceScansListHidesEarlierEnrollment checks that re-enrolling a deleted
// device's ID with a new key does not expose the scans kept from the earlier
// enrollment, while scans uploaded since then stay visible.
func TestDeviceScansListHidesEarlierEnrollment(t *testing.T) {
	gatewayClient := newEnforcementTestGatewayClient(t)

	enrollDeviceScansTestDevice(t, gatewayClient, "device-a", "key-1")
	oldScan := gtypes.DeviceScan{DeviceID: "device-a"}
	if err := gatewayClient.InsertDeviceScan(t.Context(), &oldScan); err != nil {
		t.Fatalf("insert old device scan: %v", err)
	}

	// An admin deletes the device but keeps its scans; the ID is then enrolled
	// again with a different identity key.
	if err := gatewayClient.DeleteDevice(t.Context(), "device-a", false); err != nil {
		t.Fatalf("delete device: %v", err)
	}
	enrollDeviceScansTestDevice(t, gatewayClient, "device-a", "key-2")

	newScan := gtypes.DeviceScan{DeviceID: "device-a"}
	if err := gatewayClient.InsertDeviceScan(t.Context(), &newScan); err != nil {
		t.Fatalf("insert new device scan: %v", err)
	}

	device := &user.DefaultInfo{
		Name:   gtypes.DevicePrincipalName("device-a"),
		UID:    gtypes.DevicePrincipalName("device-a"),
		Groups: []string{types.GroupDeviceScans, types.GroupAuthenticated},
		Extra: map[string][]string{
			"device_id": {"device-a"},
		},
	}

	// Both the flat list and the GroupByDevice subquery must apply the bound.
	for _, query := range []string{"?group_by_device=false", ""} {
		got := listDeviceScanIDs(t, gatewayClient, device, query)
		if !slices.Equal(got, []uint{newScan.ID}) {
			t.Fatalf("List(%q) scan IDs = %v, want only the new enrollment's scan %d", query, got, newScan.ID)
		}
	}
}

// enrollDeviceScansTestDevice enrolls deviceID with publicKey as a placeholder
// key. Enrollment only compares key bytes and never parses them, and these
// tests build the device principal directly instead of verifying a JWT, so the
// key doesn't need to be real. A different publicKey models a new identity key.
func enrollDeviceScansTestDevice(t *testing.T, c *gatewayclient.Client, deviceID, publicKey string) {
	t.Helper()

	if _, err := c.EnrollDevice(t.Context(), gatewayclient.DeviceEnrollment{
		DeviceID:           deviceID,
		MDMConfigurationID: 1,
		PublicKey:          []byte(publicKey),
	}, gatewayclient.DeviceLimit{Unlimited: true}); err != nil {
		t.Fatalf("enroll device %q: %v", deviceID, err)
	}
}

// listDeviceScanIDs calls List as u and returns the sorted IDs of the scans it wrote.
func listDeviceScanIDs(t *testing.T, c *gatewayclient.Client, u user.Info, query string) []uint {
	t.Helper()

	recorder := httptest.NewRecorder()
	req := api.Context{
		ResponseWriter: recorder,
		Request:        httptest.NewRequest(http.MethodGet, "/api/devices/scans"+query, nil),
		GatewayClient:  c,
		User:           u,
	}

	if err := (&DeviceScansHandler{}).List(req); err != nil {
		t.Fatalf("List() error = %v", err)
	}

	var resp types.DeviceScanResponse
	if err := json.NewDecoder(recorder.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	got := make([]uint, 0, len(resp.Items))
	for _, item := range resp.Items {
		got = append(got, item.ID)
	}
	slices.Sort(got)
	return got
}

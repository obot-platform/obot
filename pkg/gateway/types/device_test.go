package types

import (
	"testing"

	"k8s.io/apiserver/pkg/authentication/user"
)

func TestPrincipalDeviceID(t *testing.T) {
	tests := []struct {
		name string
		user user.Info
		want string
	}{
		{
			name: "enrolled device principal",
			user: &user.DefaultInfo{
				UID: DevicePrincipalName("device-a"),
				Extra: map[string][]string{
					"device_id": {"device-a"},
				},
			},
			want: "device-a",
		},
		{
			name: "human UID with device_id extra",
			user: &user.DefaultInfo{
				UID: "1",
				Extra: map[string][]string{
					"device_id": {"device-a"},
				},
			},
			want: "",
		},
		{
			name: "device UID for a different device",
			user: &user.DefaultInfo{
				UID: DevicePrincipalName("device-b"),
				Extra: map[string][]string{
					"device_id": {"device-a"},
				},
			},
			want: "",
		},
		{
			name: "device UID without device_id extra",
			user: &user.DefaultInfo{
				UID: DevicePrincipalName("device-a"),
			},
			want: "",
		},
		{
			name: "empty device_id extra",
			user: &user.DefaultInfo{
				UID: DevicePrincipalName(""),
				Extra: map[string][]string{
					"device_id": {""},
				},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PrincipalDeviceID(tt.user); got != tt.want {
				t.Fatalf("PrincipalDeviceID() = %q, want %q", got, tt.want)
			}
		})
	}
}

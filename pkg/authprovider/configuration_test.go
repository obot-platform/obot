package authprovider

import (
	"testing"

	clienttypes "github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestDaemonConfigurationHash(t *testing.T) {
	provider := v1.AuthProvider{
		Name: "provider",
		UID:  types.UID("uid-one"),
		Spec: v1.AuthProviderSpec{
			AuthProviderManifest: clienttypes.AuthProviderManifest{
				CommonProviderMetadata: clienttypes.CommonProviderMetadata{
					Command: "provider-command",
					Args:    []string{"serve"},
				},
			},
		},
	}

	base, err := DaemonConfigurationHash(provider, map[string]string{
		"CLIENT_ID":     "client-id",
		"CLIENT_SECRET": "client-secret",
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("map insertion order is stable", func(t *testing.T) {
		got, err := DaemonConfigurationHash(provider, map[string]string{
			"CLIENT_SECRET": "client-secret",
			"CLIENT_ID":     "client-id",
		})
		if err != nil {
			t.Fatal(err)
		}
		if got != base {
			t.Fatalf("hash = %q, want %q", got, base)
		}
	})

	t.Run("status and unrelated metadata are excluded", func(t *testing.T) {
		changed := *provider.DeepCopy()
		changed.ResourceVersion = "new-version"
		changed.Generation = 42
		changed.Annotations = map[string]string{"unrelated": "value"}
		changed.Status.Configured = true

		got, err := DaemonConfigurationHash(changed, map[string]string{
			"CLIENT_ID":     "client-id",
			"CLIENT_SECRET": "client-secret",
		})
		if err != nil {
			t.Fatal(err)
		}
		if got != base {
			t.Fatalf("hash = %q, want %q", got, base)
		}
	})

	tests := []struct {
		name       string
		provider   v1.AuthProvider
		credential map[string]string
	}{
		{
			name:     "UID change",
			provider: provider,
			credential: map[string]string{
				"CLIENT_ID":     "client-id",
				"CLIENT_SECRET": "client-secret",
			},
		},
		{
			name:     "spec change",
			provider: provider,
			credential: map[string]string{
				"CLIENT_ID":     "client-id",
				"CLIENT_SECRET": "client-secret",
			},
		},
		{
			name:     "credential change",
			provider: provider,
			credential: map[string]string{
				"CLIENT_ID":     "client-id",
				"CLIENT_SECRET": "new-client-secret",
			},
		},
	}
	tests[0].provider.UID = types.UID("uid-two")
	tests[1].provider.Spec.Args = []string{"serve", "--changed"}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DaemonConfigurationHash(tt.provider, tt.credential)
			if err != nil {
				t.Fatal(err)
			}
			if got == base {
				t.Fatalf("hash did not change from %q", base)
			}
		})
	}
}

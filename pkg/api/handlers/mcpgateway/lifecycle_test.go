package mcpgateway

import (
	"testing"
	"time"

	"k8s.io/apiserver/pkg/authentication/user"
)

func TestCompositeLoopbackTokenRecordsCaller(t *testing.T) {
	now := time.Now()

	for _, tt := range []struct {
		name   string
		caller user.Info
	}{
		{
			name: "person",
			caller: &user.DefaultInfo{
				Name: "alice",
				UID:  "7",
				Extra: map[string][]string{
					"email": {"alice@example.com"},
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := compositeLoopbackTokenContext(tt.caller, "https://obot.example.com/mcp-connect-composite/vmcp1", "vmcp1", []string{"component"}, now)

			if got.UserID != tt.caller.GetUID() {
				t.Errorf("token user = %q, want the caller %q", got.UserID, tt.caller.GetUID())
			}
		})
	}
}

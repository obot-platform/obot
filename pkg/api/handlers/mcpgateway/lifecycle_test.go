package mcpgateway

import (
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/principal"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestCompositeLoopbackTokenRecordsAHostedAgentsOwner(t *testing.T) {
	now := time.Now()

	for _, tt := range []struct {
		name      string
		caller    user.Info
		wantOwner string
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
		{
			name: "hosted agent",
			caller: &user.DefaultInfo{
				Name: "hosted-agent:hai1abc",
				UID:  "hosted-agent:hai1abc",
				Extra: map[string][]string{
					principal.HostedAgentOwnerExtra: {"7"},
				},
			},
			wantOwner: "7",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := compositeLoopbackTokenContext(tt.caller, "https://obot.example.com/mcp-connect-composite/vmcp1", "vmcp1", []string{"component"}, now)

			if got.UserID != tt.caller.GetUID() {
				t.Errorf("token user = %q, want the caller %q", got.UserID, tt.caller.GetUID())
			}
			if got.HostedAgentOwnerID != tt.wantOwner {
				t.Errorf("token hosted agent owner = %q, want %q", got.HostedAgentOwnerID, tt.wantOwner)
			}
		})
	}
}

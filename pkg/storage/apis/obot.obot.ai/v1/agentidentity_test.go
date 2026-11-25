package v1

import (
	"testing"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/stretchr/testify/require"
)

// TestAgentIdentityEventFields checks that agent lifecycle events are selected by the agent's bare decimal ID, the ID
// in its principal, so that a controller watching an agent's events never selects a user's.
func TestAgentIdentityEventFields(t *testing.T) {
	for _, tc := range []struct {
		name   string
		object fields.Fields
		want   string
	}{
		{
			name:   "change",
			object: &AgentIdentityChange{Spec: AgentIdentityChangeSpec{AgentID: 42}},
			want:   "42",
		},
		{
			name:   "delete",
			object: &AgentIdentityDelete{Spec: AgentIdentityDeleteSpec{AgentID: 42}},
			want:   "42",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, []string{"spec.agentID"}, tc.object.FieldNames())
			require.True(t, tc.object.Has("spec.agentID"))
			require.Equal(t, tc.want, tc.object.Get("spec.agentID"))
			require.False(t, tc.object.Has("spec.userID"))
			require.Empty(t, tc.object.Get("spec.userID"))
		})
	}
}

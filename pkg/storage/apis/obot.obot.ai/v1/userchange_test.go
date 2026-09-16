package v1

import (
	"testing"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/stretchr/testify/require"
)

func TestUserChangeFields(t *testing.T) {
	for _, tc := range []struct {
		name   string
		object fields.Fields
		want   string
	}{
		{
			name:   "group change",
			object: &UserGroupChange{Spec: UserGroupChangeSpec{UserID: 42}},
			want:   "42",
		},
		{
			name:   "role change",
			object: &UserRoleChange{Spec: UserRoleChangeSpec{UserID: 42}},
			want:   "42",
		},
		{
			name:   "zero group user ID",
			object: &UserGroupChange{},
			want:   "0",
		},
		{
			name:   "zero role user ID",
			object: &UserRoleChange{},
			want:   "0",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, []string{"spec.userID"}, tc.object.FieldNames())
			require.True(t, tc.object.Has("spec.userID"))
			require.Equal(t, tc.want, tc.object.Get("spec.userID"))
			require.False(t, tc.object.Has("unknown"))
			require.Empty(t, tc.object.Get("unknown"))
		})
	}
}

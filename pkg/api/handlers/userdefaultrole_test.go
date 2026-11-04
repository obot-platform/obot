package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
)

func TestUserDefaultRoleSettingRejectsPrivilegedRoles(t *testing.T) {
	tests := []struct {
		name string
		role types.Role
	}{
		{
			name: "owner",
			role: types.RoleOwner,
		},
		{
			name: "admin read-only",
			role: types.RoleAdminReadonly,
		},
		{
			name: "basic with auditor",
			role: types.RoleBasic | types.RoleAuditor,
		},
		{
			name: "unknown",
			role: 1 << 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(types.UserDefaultRoleSetting{Role: tt.role})
			if err != nil {
				t.Fatalf("marshal setting: %v", err)
			}
			req := api.Context{
				ResponseWriter: httptest.NewRecorder(),
				Request:        httptest.NewRequest(http.MethodPost, "/api/user-default-role-settings", bytes.NewReader(body)),
			}

			// Rejected roles fail before storage is read, so the context needs none.
			err = (*UserDefaultRoleSettingHandler)(nil).Set(req)
			var httpErr *types.ErrHTTP
			if !errors.As(err, &httpErr) || httpErr.Code != http.StatusBadRequest {
				t.Fatalf("Set() error = %v, want a bad request", err)
			}
		})
	}
}

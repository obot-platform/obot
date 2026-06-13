package oidcjwt

import (
	"net/http"
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

type RoleUplift struct {
	next       authenticator.Request
	adminRoles map[string]struct{}
}

func NewRoleUplift(next authenticator.Request, cfg Config) *RoleUplift {
	adminRoles := make(map[string]struct{}, len(cfg.AdminRoles))
	for _, role := range cfg.AdminRoles {
		adminRoles[role] = struct{}{}
	}
	return &RoleUplift{next: next, adminRoles: adminRoles}
}

func (r *RoleUplift) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	resp, ok, err := r.next.AuthenticateRequest(req)
	if err != nil || !ok || resp == nil || resp.User == nil {
		return resp, ok, err
	}
	if !r.hasAdminRole(resp.User.GetExtra()[jwtRolesExtraKey]) {
		return resp, ok, nil
	}

	groups := append([]string{}, resp.User.GetGroups()...)
	for _, group := range types.RoleAdmin.Groups() {
		if group == types.GroupOwner || slices.Contains(groups, group) {
			continue
		}
		groups = append(groups, group)
	}

	resp.User = &user.DefaultInfo{
		Name:   resp.User.GetName(),
		UID:    resp.User.GetUID(),
		Groups: groups,
		Extra:  resp.User.GetExtra(),
	}
	return resp, ok, nil
}

func (r *RoleUplift) hasAdminRole(jwtRoles []string) bool {
	for _, role := range jwtRoles {
		if _, ok := r.adminRoles[role]; ok {
			return true
		}
	}
	return false
}

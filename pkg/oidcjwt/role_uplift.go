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
	ownerRoles map[string]struct{}
}

func NewRoleUplift(next authenticator.Request, cfg Config) *RoleUplift {
	adminRoles := make(map[string]struct{}, len(cfg.AdminRoles))
	for _, role := range cfg.AdminRoles {
		adminRoles[role] = struct{}{}
	}
	ownerRoles := make(map[string]struct{}, len(cfg.OwnerRoles))
	for _, role := range cfg.OwnerRoles {
		ownerRoles[role] = struct{}{}
	}
	return &RoleUplift{next: next, adminRoles: adminRoles, ownerRoles: ownerRoles}
}

func (r *RoleUplift) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	resp, ok, err := r.next.AuthenticateRequest(req)
	if err != nil || !ok || resp == nil || resp.User == nil {
		return resp, ok, err
	}
	jwtRoles, authoritative := resp.User.GetExtra()[jwtRolesExtraKey]
	if !authoritative {
		return resp, ok, nil
	}

	var claimedGroups []string
	if r.hasRole(jwtRoles, r.ownerRoles) {
		claimedGroups = types.RoleOwner.Groups()
	} else if r.hasRole(jwtRoles, r.adminRoles) {
		claimedGroups = types.RoleAdmin.Groups()
	}

	baseRoleGroups := []string{
		types.GroupOwner,
		types.GroupAdmin,
		types.GroupPowerUserPlus,
		types.GroupPowerUser,
	}
	groups := make([]string, 0, len(resp.User.GetGroups())+len(claimedGroups))
	for _, group := range resp.User.GetGroups() {
		if !slices.Contains(baseRoleGroups, group) {
			groups = append(groups, group)
		}
	}
	for _, group := range claimedGroups {
		if slices.Contains(groups, group) {
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

func (r *RoleUplift) hasRole(jwtRoles []string, configured map[string]struct{}) bool {
	for _, role := range jwtRoles {
		if _, ok := configured[role]; ok {
			return true
		}
	}
	return false
}

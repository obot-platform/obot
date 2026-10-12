package clauderunner

import (
	"context"
	"errors"
	"slices"
	"strconv"

	api "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/vmcp"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/api/resource"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

// ResolveEnvironment combines matching grants without granting any vMCP outside
// the environment allowlist. Profiles inherit each unspecified numeric value.
func ResolveEnvironment(user kuser.Info, env *types.ClaudeEnvironment) (*types.ClaudeEnvironment, error) {
	resolved := *env
	resolved.MCPServerIDs = []string{}
	resolved.Profiles = nil
	resolved.EgressSecrets = slices.Clone(env.EgressSecrets)
	secretIndexes := map[string]int{}
	for i, secret := range resolved.EgressSecrets {
		secretIndexes[SecretKey(secret)] = i
	}
	overridden := map[string]bool{}
	resolved.Runtime.EgressHosts = slices.Clone(env.Runtime.EgressHosts)
	resolved.Runtime.Resources = types.ClaudeResources{}
	matched := false
	for _, profile := range env.Profiles {
		if len(vmcp.MatchingProfiles(user, []api.VMCPProfile{{Subjects: profile.Subjects}})) == 0 {
			continue
		}
		matched = true
		for _, secret := range profile.EgressSecrets {
			key := SecretKey(secret)
			if overridden[key] {
				continue
			}
			overridden[key] = true
			if index, exists := secretIndexes[key]; exists {
				resolved.EgressSecrets[index] = secret
			} else {
				secretIndexes[key] = len(resolved.EgressSecrets)
				resolved.EgressSecrets = append(resolved.EgressSecrets, secret)
			}
		}

		ids := profile.MCPServerIDs
		if ids == nil {
			ids = env.MCPServerIDs
		}
		for _, id := range ids {
			if slices.Contains(env.MCPServerIDs, id) && !slices.Contains(resolved.MCPServerIDs, id) {
				resolved.MCPServerIDs = append(resolved.MCPServerIDs, id)
			}
		}

		resolved.Runtime.AllowAllEgress = resolved.Runtime.AllowAllEgress || profile.AllowAllEgress
		for _, host := range profile.EgressHosts {
			if !slices.Contains(resolved.Runtime.EgressHosts, host) {
				resolved.Runtime.EgressHosts = append(resolved.Runtime.EgressHosts, host)
			}
		}

		r := EffectiveResources(env.Runtime.Resources, profile.Resources)
		target := &resolved.Runtime.Resources
		target.ActorCPU = maxQuantity(target.ActorCPU, r.ActorCPU)
		target.ActorMemory = maxQuantity(target.ActorMemory, r.ActorMemory)
		target.IdleTimeoutMinutes = max(target.IdleTimeoutMinutes, r.IdleTimeoutMinutes)
		if target.SessionRetentionDays == -1 || r.SessionRetentionDays == -1 {
			target.SessionRetentionDays = -1
		} else {
			target.SessionRetentionDays = max(target.SessionRetentionDays, r.SessionRetentionDays)
		}
	}
	if !matched {
		return nil, gorm.ErrRecordNotFound
	}
	slices.Sort(resolved.MCPServerIDs)
	slices.Sort(resolved.Runtime.EgressHosts)
	return &resolved, nil
}

func EffectiveResources(base, override types.ClaudeResources) types.ClaudeResources {
	if override.ActorCPU != "" {
		base.ActorCPU = override.ActorCPU
	}
	if override.ActorMemory != "" {
		base.ActorMemory = override.ActorMemory
	}
	if override.IdleTimeoutMinutes != 0 {
		base.IdleTimeoutMinutes = override.IdleTimeoutMinutes
	}
	if override.SessionRetentionDays != 0 {
		base.SessionRetentionDays = override.SessionRetentionDays
	}
	return base
}

func maxQuantity(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	aq, ae := resource.ParseQuantity(a)
	bq, be := resource.ParseQuantity(b)
	if ae == nil && be == nil && bq.Cmp(aq) > 0 {
		return b
	}
	return a
}

// AuthorizedEnvironment rechecks the administrator-approved identity and current
// profile membership on admission, credential refresh, and reconciliation.
func AuthorizedEnvironment(ctx context.Context, gateway *client.Client, environment, account, owner string) (*types.ClaudeEnvironment, error) {
	env, err := gateway.ClaudeEnvironment(ctx, environment)
	if err != nil {
		return nil, err
	}
	if account == ClaudeTagAccount {
		if owner == "" || !env.ClaudeTagEnabled || env.ClaudeTagUserID != owner {
			return nil, gorm.ErrRecordNotFound
		}
		serviceUser, err := gateway.UserByID(ctx, owner)
		if err != nil {
			return nil, err
		}
		if !serviceUser.Internal {
			return nil, gorm.ErrRecordNotFound
		}
	} else {
		mapping, err := gateway.ClaudeOrganizationAccountForUser(ctx, env.OrganizationID, owner)
		if err != nil {
			return nil, err
		}
		if mapping.Account != account {
			return nil, gorm.ErrRecordNotFound
		}
	}
	id, err := strconv.ParseUint(owner, 10, 64)
	if err != nil {
		return nil, err
	}
	user, err := gateway.UserInfoByID(ctx, uint(id))
	if err != nil {
		return nil, err
	}
	if status, recorded := principal.UserStatus(user); !recorded || status != api.UserStatusActive {
		return nil, gorm.ErrRecordNotFound
	}

	return ResolveEnvironment(user, env)
}

func AccessDenied(err error) bool { return errors.Is(err, gorm.ErrRecordNotFound) }

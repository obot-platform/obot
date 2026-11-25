// Package principal interprets who an authenticated caller is.
package principal

import (
	"fmt"
	"slices"
	"strconv"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/system"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

const (
	// APIKeyIDExtra and APIKeyNameExtra carry the non-secret credential
	// attribution established by the API-key authenticator. Downstream code
	// must use APIKeyAttributionFromUser instead of interpreting these values.
	APIKeyIDExtra   = "api_key_id"
	APIKeyNameExtra = "api_key_name"
	// APIKeyRawNameExtra preserves the actual name separately from the audit display fallback.
	APIKeyRawNameExtra = "api_key_raw_name"
)

// APIKeyAttribution identifies the API key that authenticated a request.
type APIKeyAttribution struct {
	ID   uint
	Name string
}

// MaskedAPIKeyName returns the non-secret identifier for an API key of p: the key prefix of p's kind, p's ID and the
// key's ID, with the secret masked.
func (p Principal) MaskedAPIKeyName(keyID uint) string {
	prefix := system.APIKeyPrefix
	if p.Kind == KindAgent {
		prefix = system.AgentAPIKeyPrefix
	}
	return fmt.Sprintf("%s-%s-%d-*****", prefix, p.ID, keyID)
}

// MaskedAPIKeyName returns Principal.MaskedAPIKeyName for an owner that is only known by its storage encoding
// (Principal.String): an agent's key is masked with the agent key prefix, and any other owner is written as is after
// the user key prefix.
func MaskedAPIKeyName(owner string, keyID uint) string {
	if agentID, ok := ParseAgent(owner); ok {
		return Principal{Kind: KindAgent, ID: agentID}.MaskedAPIKeyName(keyID)
	}
	return Principal{Kind: KindUser, ID: owner}.MaskedAPIKeyName(keyID)
}

// NewAPIKeyAttribution creates event-time display attribution for an API key.
// Unnamed keys use the reconstructable, non-secret masked key identifier so
// audit consumers never need to join back to the API-key table to label them.
func NewAPIKeyAttribution(id uint, owner Principal, name string) APIKeyAttribution {
	if name == "" {
		name = owner.MaskedAPIKeyName(id)
	}
	return APIKeyAttribution{ID: id, Name: name}
}

// APIKeyAttributionFromUser returns the API key that authenticated a caller.
// A missing or malformed ID means the caller did not authenticate with an API
// key. The authenticator supplies a display name, including the masked fallback
// for an unnamed key; an empty name indicates a legacy or malformed principal.
func APIKeyAttributionFromUser(user kuser.Info) (APIKeyAttribution, bool) {
	if user == nil {
		return APIKeyAttribution{}, false
	}
	extra := user.GetExtra()
	ids := extra[APIKeyIDExtra]
	if len(ids) == 0 {
		return APIKeyAttribution{}, false
	}
	id, err := strconv.ParseUint(ids[0], 10, 0)
	if err != nil || id == 0 {
		return APIKeyAttribution{}, false
	}
	attribution := APIKeyAttribution{ID: uint(id)}
	if names := extra[APIKeyNameExtra]; len(names) > 0 {
		attribution.Name = names[0]
	}
	return attribution, true
}

// IsAdminOrOwner returns whether the caller is an admin or owner. An owner has the admin group too, so that's the
// one to look for. An API key carries only the groups of its scopes, so the caller's role is also read from
// obot_groups: an LLM-only key of an admin has no admin group.
func IsAdminOrOwner(user kuser.Info) bool {
	return slices.Contains(user.GetGroups(), types.GroupAdmin) ||
		slices.Contains(user.GetExtra()["obot_groups"], types.GroupAdmin)
}

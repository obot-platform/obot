// Package principal interprets who an authenticated caller is.
package principal

import (
	"fmt"
	"strconv"

	"github.com/obot-platform/obot/pkg/system"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

const (
	// APIKeyIDExtra and APIKeyNameExtra carry the non-secret credential
	// attribution established by the API-key authenticator. Downstream code
	// must use APIKeyAttributionFromUser instead of interpreting these values.
	APIKeyIDExtra   = "api_key_id"
	APIKeyNameExtra = "api_key_name"
)

// APIKeyAttribution identifies the API key that authenticated a request.
type APIKeyAttribution struct {
	ID   uint
	Name string
}

// MaskedAPIKeyName returns the non-secret identifier for an API key.
func MaskedAPIKeyName(userID string, keyID uint) string {
	return fmt.Sprintf("%s-%s-%d-*****", system.APIKeyPrefix, userID, keyID)
}

// NewAPIKeyAttribution creates event-time display attribution for an API key.
// Unnamed keys use the reconstructable, non-secret masked key identifier so
// audit consumers never need to join back to the API-key table to label them.
func NewAPIKeyAttribution(id, ownerUserID uint, name string) APIKeyAttribution {
	if name == "" {
		name = MaskedAPIKeyName(strconv.FormatUint(uint64(ownerUserID), 10), id)
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

package auth

import (
	"context"
	"time"
)

// SerializableRequest represents an HTTP request that can be serialized for authentication flows
type SerializableRequest struct {
	Method string              `json:"method"`
	URL    string              `json:"url"`
	Header map[string][]string `json:"header"`
}

// GroupInfo represents information about a user group from an authentication provider
type GroupInfo struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	IconURL *string `json:"iconURL"`
}

// SerializableState represents the authentication state returned from auth providers
type SerializableState struct {
	ExpiresOn         *time.Time  `json:"expiresOn"`
	AccessToken       string      `json:"accessToken"`
	PreferredUsername string      `json:"preferredUsername"`
	User              string      `json:"user"`
	Email             string      `json:"email"`
	Groups            []string    `json:"groups"`
	GroupInfos        []GroupInfo `json:"groupInfos"`
	SetCookies        []string    `json:"setCookies"`
}

type groupInfoKey struct{}

// ContextWithGroupInfos adds group information to the context
func ContextWithGroupInfos(ctx context.Context, groupInfos []GroupInfo) context.Context {
	return context.WithValue(ctx, groupInfoKey{}, groupInfos)
}

// GroupInfosFromContext retrieves group information from the context
func GroupInfosFromContext(ctx context.Context) []GroupInfo {
	groupInfos, ok := ctx.Value(groupInfoKey{}).([]GroupInfo)
	if !ok {
		return nil
	}
	return groupInfos
}

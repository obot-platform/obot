package mcpgateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"unicode/utf8"

	"github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	"golang.org/x/net/http/httpguts"
	"k8s.io/apiserver/pkg/authentication/user"
)

// Bound each attribution value independently.
const maxObotHeaderValueCharacters = 512

// identityReader keeps the database lookup local to requests that select the IdP ID.
type identityReader interface {
	FindIdentitiesForUser(context.Context, uint) ([]gatewaytypes.Identity, error)
}

// callerObotHeaders derives only the fields explicitly selected by the destination.
// It is request-local: caller identity must never enter a shared deployment or client.
func callerObotHeaders(runtime types.Runtime, selected []types.ObotHeader, caller user.Info, idpUserID string) (http.Header, error) {
	headers := make(http.Header)
	if caller == nil || !types.SupportsObotHeaders(runtime) {
		return headers, nil
	}
	values := map[types.ObotHeader]string{
		types.ObotHeaderIDPUserID: idpUserID,
		types.ObotHeaderUserID:    caller.GetUID(),
		types.ObotHeaderUserName:  caller.GetName(),
		types.ObotHeaderUserEmail: firstExtra(caller.GetExtra(), "email"),
	}
	if key, ok := principal.APIKeyAttributionFromUser(caller); ok {
		values[types.ObotHeaderAPIKeyID] = strconv.FormatUint(uint64(key.ID), 10)
		// The audit name may be a masked identifier. Never disclose it as a raw key name.
		values[types.ObotHeaderAPIKeyName] = firstExtra(caller.GetExtra(), principal.APIKeyRawNameExtra)
	}
	for _, name := range selected {
		header, ok := types.CanonicalObotHeader(string(name))
		if !ok {
			return nil, fmt.Errorf("unsupported injectObotHeaders header %q", name)
		}
		value := values[header]
		if header == types.ObotHeaderIDPGroups {
			groups := slices.Clone(caller.GetExtra()["auth_provider_groups"])
			groups = slices.DeleteFunc(groups, func(group string) bool { return group == "" })
			if len(groups) > 0 {
				slices.Sort(groups)
				encoded, err := json.Marshal(slices.Compact(groups))
				if err != nil {
					return nil, fmt.Errorf("encode IdP groups: %w", err)
				}
				value = string(encoded)
			}
		}
		if value == "" {
			continue
		}
		if !httpguts.ValidHeaderFieldValue(value) {
			slog.Warn("Omitting MCP attribution header", "header", header, "reason", "invalid header value")
			continue
		}
		if utf8.RuneCountInString(value) > maxObotHeaderValueCharacters {
			slog.Warn("Omitting MCP attribution header", "header", header, "reason", "header size limit exceeded")
			continue
		}
		headers.Set(string(header), value)
	}
	return headers, nil
}

func selectedIDPUserID(ctx context.Context, reader identityReader, runtime types.Runtime, selected []types.ObotHeader, caller user.Info) (string, error) {
	if caller == nil || !types.SupportsObotHeaders(runtime) {
		return "", nil
	}
	selectedID := false
	for _, name := range selected {
		header, _ := types.CanonicalObotHeader(string(name))
		selectedID = selectedID || header == types.ObotHeaderIDPUserID
	}
	if !selectedID {
		return "", nil
	}
	userID, err := strconv.ParseUint(caller.GetUID(), 10, 64)
	if err != nil {
		return "", nil
	}
	identities, err := reader.FindIdentitiesForUser(ctx, uint(userID))
	if err != nil {
		return "", fmt.Errorf("resolve IdP user ID: %w", err)
	}
	providerName := firstExtra(caller.GetExtra(), "auth_provider_name")
	providerNamespace := firstExtra(caller.GetExtra(), "auth_provider_namespace")
	var matches []gatewaytypes.Identity
	for _, identity := range identities {
		if providerName != "" && identity.AuthProviderName != providerName {
			continue
		}
		if providerNamespace != "" && identity.AuthProviderNamespace != providerNamespace {
			continue
		}
		matches = append(matches, identity)
	}
	// API keys identify an account, not a provider identity. Never pick an arbitrary identity.
	if len(matches) != 1 {
		return "", nil
	}
	return matches[0].ProviderUserID, nil
}

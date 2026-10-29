// Package attribution owns MCP caller attribution and reserves its header names.
package attribution

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	"golang.org/x/net/http/httpguts"
	"k8s.io/apiserver/pkg/authentication/user"
)

// Bound each attribution value independently.
const (
	maxObotHeaderValueCharacters = 512
)

// IdentityReader supplies stored identities for the authenticated caller.
// It is only consulted when the destination selects the IdP user ID.
type IdentityReader interface {
	FindIdentitiesForUser(context.Context, uint) ([]gatewaytypes.Identity, error)
}

// ForCaller derives only the attribution fields selected by the destination,
// resolving the caller's provider identity only when needed. Returned headers
// belong to this request and must not be stored in a shared client or deployment.
func ForCaller(ctx context.Context, reader IdentityReader, runtime types.Runtime, selected []types.ObotHeader, caller user.Info) (http.Header, error) {
	idpUserID, err := selectedIDPUserID(ctx, reader, runtime, selected, caller)
	if err != nil {
		return nil, err
	}
	headers, err := callerHeaders(runtime, selected, caller, idpUserID)
	if err != nil {
		return nil, types.NewErrBadRequest("%v", err)
	}
	return headers, nil
}

// callerHeaders derives only the fields explicitly selected by the destination.
// It is request-local: caller identity must never enter a shared deployment or client.
func callerHeaders(runtime types.Runtime, selected []types.ObotHeader, caller user.Info, idpUserID string) (http.Header, error) {
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

func selectedIDPUserID(ctx context.Context, reader IdentityReader, runtime types.Runtime, selected []types.ObotHeader, caller user.Info) (string, error) {
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

// Strip removes all reserved attribution names, including non-canonical map keys
// supplied by in-process callers. Apply it before consuming incoming headers.
func Strip(headers http.Header) {
	for name := range headers {
		if _, reserved := types.CanonicalObotHeader(name); reserved {
			delete(headers, name)
		}
	}
}

// Configured combines per-user and static headers, excluding reserved attribution
// names. Static values take precedence. Copying the result onto an outgoing request
// cannot overwrite or remove attribution already derived for its caller.
func Configured(static, keys, values []string) http.Header {
	headers := make(http.Header, len(keys)+len(static))
	set := func(name, value string) {
		if _, reserved := types.CanonicalObotHeader(name); !reserved {
			headers.Set(name, value)
		}
	}
	for i, key := range keys {
		if i < len(values) {
			set(key, values[i])
		}
	}
	for _, header := range static {
		if key, value, ok := strings.Cut(header, "="); ok {
			set(key, value)
		}
	}
	return headers
}

func firstExtra(extra map[string][]string, key string) string {
	if values := extra[key]; len(values) > 0 {
		return values[0]
	}
	return ""
}

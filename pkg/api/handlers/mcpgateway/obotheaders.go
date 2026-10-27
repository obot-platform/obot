package mcpgateway

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/principal"
	"golang.org/x/net/http/httpguts"
	"k8s.io/apiserver/pkg/authentication/user"
)

// callerObotHeaders derives only the fields explicitly selected by the destination.
// It is request-local: caller identity must never enter a shared deployment or client.
func callerObotHeaders(runtime types.Runtime, selected []types.ObotHeader, caller user.Info) (http.Header, error) {
	headers := make(http.Header)
	if caller == nil || (runtime != types.RuntimeRemote && runtime != types.RuntimeContainerized) {
		return headers, nil
	}
	values := map[types.ObotHeader]string{
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
		if value == "" {
			continue
		}
		if !httpguts.ValidHeaderFieldValue(value) {
			return nil, fmt.Errorf("invalid value for attribution header %s", header)
		}
		headers.Set(string(header), value)
	}
	return headers, nil
}

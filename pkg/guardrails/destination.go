package guardrails

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/obot-platform/obot/pkg/safehttp"
)

var (
	// destinationPolicy is where guardrails may send content and credentials. A guardrail service
	// may be on a private network, such as a self-hosted Presidio, but not on a loopback address,
	// which would reach services on the Obot host itself, nor on a link-local or cloud metadata
	// address, which would reach the host's cloud credentials. It is checked when a guardrail is
	// saved and again on every connection, so a host name that later resolves elsewhere is
	// refused too.
	destinationPolicy = safehttp.Options{
		BlockLoopback:  true,
		BlockLinkLocal: true,
	}
)

// newHTTPClient returns the client guardrails use for external services.
func newHTTPClient() *http.Client {
	options := destinationPolicy
	options.Timeout = 30 * time.Second
	return safehttp.NewClient(options)
}

// CheckDestinations refuses guardrail settings that send content or credentials to an address
// guardrails may not use (see destinationPolicy). Only addresses known to be refused are
// reported: one that doesn't resolve yet is left for the guardrail to report when it's used.
func CheckDestinations(ctx context.Context, typ string, config map[string]any) error {
	info, ok := Provider(typ)
	if !ok {
		return nil
	}
	for _, f := range info.Fields {
		value, _ := config[f.Name].(string)
		if !f.Destination || !strings.Contains(value, "://") {
			continue
		}
		var blocked *safehttp.BlockedAddressError
		if err := safehttp.ValidateURL(ctx, strings.TrimSpace(value), destinationPolicy); errors.As(err, &blocked) {
			return fmt.Errorf("%s %s can't be used: %s is a %s address; guardrail services may be on a private network, but not on a loopback, link-local or cloud metadata address", f.Label, value, blocked.IP, blocked.Reason)
		}
	}
	return nil
}

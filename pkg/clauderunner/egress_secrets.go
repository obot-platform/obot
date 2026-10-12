package clauderunner

import (
	"strings"

	"github.com/obot-platform/obot/pkg/gateway/types"
)

// EgressHostAllowed uses Substrate's single-label wildcard matching.
func EgressHostAllowed(runtime types.ClaudeRuntime, host string) bool {
	if runtime.AllowAllEgress || host == "api.anthropic.com" {
		return true
	}
	for _, pattern := range runtime.EgressHosts {
		if pattern == host {
			return true
		}
		if suffix, ok := strings.CutPrefix(pattern, "*."); ok {
			first, rest, found := strings.Cut(host, ".")
			if found && first != "" && rest == suffix {
				return true
			}
		}
	}
	return false
}

func SecretKey(secret types.ClaudeEgressSecret) string {
	return secret.Hostname + "/" + strings.ToLower(secret.Header)
}

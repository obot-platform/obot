package handlers

import (
	"strings"

	api "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/clauderunner"
	gt "github.com/obot-platform/obot/pkg/gateway/types"
	"golang.org/x/net/http/httpguts"
)

// Blank values retain the existing secret by ID; omitting an entry deletes it.
func prepareClaudeEgressSecrets(input, previous *gt.ClaudeEnvironment) error {
	old := map[string]gt.ClaudeEgressSecret{}
	for _, secret := range previous.EgressSecrets {
		old[secret.ID] = secret
	}
	for _, profile := range previous.Profiles {
		for _, secret := range profile.EgressSecrets {
			old[secret.ID] = secret
		}
	}
	seen := map[string]bool{}
	headersByHost := map[string]map[string]bool{}
	prepare := func(secrets []gt.ClaudeEgressSecret) error {
		pairs := map[string]bool{}
		for i := range secrets {
			secret := &secrets[i]
			secret.Hostname = strings.TrimSpace(secret.Hostname)
			secret.Header = strings.ToLower(strings.TrimSpace(secret.Header))
			if strings.Contains(secret.Hostname, "*") || clauderunner.ValidateEgressHosts([]string{secret.Hostname}) != nil {
				return api.NewErrBadRequest("Secret destinations must be exact lowercase DNS hostnames")
			}
			if !httpguts.ValidHeaderFieldName(secret.Header) || reservedSecretHeader(secret.Header) {
				return api.NewErrBadRequest("Secret header name is invalid or reserved")
			}
			key := clauderunner.SecretKey(*secret)
			if pairs[key] {
				return api.NewErrBadRequest("Duplicate secret destination and header")
			}
			pairs[key] = true
			if headersByHost[secret.Hostname] == nil {
				headersByHost[secret.Hostname] = map[string]bool{}
			}
			headersByHost[secret.Hostname][secret.Header] = true
			if len(headersByHost[secret.Hostname]) > 16 {
				return api.NewErrBadRequest("At most 16 distinct secret headers per destination are allowed across the environment and profiles")
			}
			if secret.ID == "" {
				secret.ID = "ces-" + randomClaudeToken()[:24]
			} else if _, ok := old[secret.ID]; !ok {
				return api.NewErrBadRequest("Unknown egress secret ID")
			}
			if seen[secret.ID] {
				return api.NewErrBadRequest("Duplicate egress secret ID")
			}
			seen[secret.ID] = true
			if secret.Value == "" {
				secret.Value = old[secret.ID].Value
			}
			if saved, ok := old[secret.ID]; ok && clauderunner.SecretKey(saved) != key {
				// A stale actor policy must not send the new secret to its previous destination.
				secret.ID = "ces-" + randomClaudeToken()[:24]
			}
			if secret.Value == "" || len(secret.Value) > 8192 || !httpguts.ValidHeaderFieldValue(secret.Value) {
				return api.NewErrBadRequest("Secret values must be nonempty HTTP header values of at most 8192 bytes")
			}
		}
		return nil
	}
	if err := prepare(input.EgressSecrets); err != nil {
		return err
	}
	for i := range input.Profiles {
		if err := prepare(input.Profiles[i].EgressSecrets); err != nil {
			return err
		}
	}
	if len(seen) > 100 {
		return api.NewErrBadRequest("At most 100 egress secrets are allowed across the environment and profiles")
	}
	return nil
}

func reservedSecretHeader(header string) bool {
	if strings.HasPrefix(header, "x-obot-") || strings.HasPrefix(header, "x-forwarded-") || strings.HasPrefix(header, "proxy-") {
		return true
	}
	switch header {
	case "host", "connection", "content-length", "transfer-encoding", "te", "trailer", "upgrade", "expect", "forwarded", "content-type", "content-encoding", "accept", "user-agent":
		return true
	}
	return false
}

func redactClaudeEgressSecrets(environment *gt.ClaudeEnvironment) {
	for i := range environment.EgressSecrets {
		environment.EgressSecrets[i].Value = ""
	}
	for i := range environment.Profiles {
		for j := range environment.Profiles[i].EgressSecrets {
			environment.Profiles[i].EgressSecrets[j].Value = ""
		}
	}
}

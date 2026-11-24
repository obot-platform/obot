package vmcp

import (
	"github.com/obot-platform/obot/apiclient/types"
)

// An agent never enters configuration, and nothing enters it for an agent. Its component servers get only the values
// fixed for everyone: by the vMCP (the fixed policy, with a value or a Secret) or by the component's catalog entry (a
// static value or a Secret binding). A value that a user would enter on their own connection (the userAllowed policy)
// is unset for an agent. An optional one is simply left out. A required one, including the server URL (__url) of a
// hostname-constrained remote that the vMCP does not fix, keeps the agent from the component, and so from the whole
// vMCP, until an administrator fixes the value on the vMCP or the catalog entry, or stops granting the component to
// the agent in every profile that names it.

// AgentMissingConfiguration returns, sorted, the required configuration keys of component that an agent cannot be
// given (see above): those a user would enter on their own connection (the userAllowed policy) and that the catalog
// entry does not supply itself, as a literal (Static) or from a Secret (SecretBinding). It reads no value, so it costs
// nothing on the connect path, and readiness and the connect path compute the same answer.
func AgentMissingConfiguration(component types.VMCPComponent) []string {
	var missing []string
	for _, key := range MissingRequiredConfiguration(component, nil, true) {
		if _, field, ok := ParseConfigurationKey(key); ok {
			missing = append(missing, field)
		}
	}
	return missing
}

// ConfigurationRequiredReasons returns a types.AgentReadinessConfigurationRequired reason for each of fields, the plain
// keys of the missing configuration of the component with componentID. It never carries a value.
func ConfigurationRequiredReasons(componentID string, fields []string) []ReadinessReason {
	reasons := make([]ReadinessReason, 0, len(fields))
	for _, field := range fields {
		reasons = append(reasons, ReadinessReason{
			ComponentID: componentID,
			Code:        types.AgentReadinessConfigurationRequired,
			Field:       field,
		})
	}
	return reasons
}

package handlers

import (
	"strings"

	types "github.com/obot-platform/obot/apiclient/types"
	gtypes "github.com/obot-platform/obot/pkg/gateway/types"
)

// defaultEnforcementAllowlist is applied to a newly created configuration that
// enables enforcement without supplying an allowlist.
func defaultEnforcementAllowlist() types.EnforcementAllowlist {
	return types.EnforcementAllowlist{
		AllowAllObotHostedMCP:     true,
		AllowAllBuiltinAgentTools: true,
		AllowAllBuiltinAgentMCP:   true,
	}
}

// enforcementAllowlistForSave normalizes and validates the incoming allowlist
// that will be stored on the configuration. When enforcement is enabled on a
// newly created configuration (current == nil) with no meaningful allowlist, the
// default is applied.
func enforcementAllowlistForSave(enabled bool, allowlist types.EnforcementAllowlist, current *gtypes.MDMConfiguration) (types.EnforcementAllowlist, error) {
	allowlist, err := normalizeEnforcementAllowlist(allowlist)
	if err != nil {
		return types.EnforcementAllowlist{}, err
	}
	if enforcementAllowlistIsEmpty(allowlist) {
		if enabled && current == nil {
			return defaultEnforcementAllowlist(), nil
		}
		return types.EnforcementAllowlist{}, nil
	}
	if err := validateEnforcementAllowlist(allowlist); err != nil {
		return types.EnforcementAllowlist{}, err
	}
	return allowlist, nil
}

func normalizeEnforcementAllowlist(allowlist types.EnforcementAllowlist) (types.EnforcementAllowlist, error) {
	if len(allowlist.Servers) == 0 {
		return allowlist, nil
	}

	// Entries are rebuilt rather than edited so neither the caller's slice nor
	// its package pointers are mutated.
	servers := make([]types.AllowlistServer, 0, len(allowlist.Servers))
	for i, server := range allowlist.Servers {
		normalized := types.AllowlistServer{
			URL:      strings.TrimSpace(server.URL),
			Hostname: strings.TrimSpace(server.Hostname),
		}
		if server.Package != nil {
			// Source is deliberately left as-is: it is matched against a closed
			// set of values, so a padded source stays an explicit error.
			normalized.Package = &types.AllowlistServerPackage{
				Source:  server.Package.Source,
				Name:    strings.TrimSpace(server.Package.Name),
				Version: strings.TrimSpace(server.Package.Version),
			}
		}
		if len(server.Tools) > 0 {
			tools := make([]string, 0, len(server.Tools))
			for _, tool := range server.Tools {
				if tool = strings.TrimSpace(tool); tool != "" {
					tools = append(tools, tool)
				}
			}
			if len(tools) == 0 {
				return types.EnforcementAllowlist{}, types.NewErrBadRequest(
					"enforcement allowlist server entry %d lists only blank tool names; omit tools entirely to allow every tool on the server", i)
			}
			normalized.Tools = tools
		}
		servers = append(servers, normalized)
	}

	allowlist.Servers = servers
	return allowlist, nil
}

func enforcementAllowlistIsEmpty(allowlist types.EnforcementAllowlist) bool {
	return !allowlist.AllowEverything &&
		!allowlist.AllowAllObotHostedMCP &&
		!allowlist.AllowAllBuiltinAgentTools &&
		!allowlist.AllowAllBuiltinAgentMCP &&
		len(allowlist.Servers) == 0
}

func validateEnforcementAllowlist(allowlist types.EnforcementAllowlist) error {
	for i, server := range allowlist.Servers {
		set := 0
		if strings.TrimSpace(server.URL) != "" {
			set++
		}
		if server.Package != nil {
			set++
		}
		if strings.TrimSpace(server.Hostname) != "" {
			set++
		}
		if set != 1 {
			return types.NewErrBadRequest("enforcement allowlist server entry %d must set exactly one of url, package, or hostname", i)
		}
		if server.Package != nil {
			switch server.Package.Source {
			case types.AllowlistServerPackageSourceNPM, types.AllowlistServerPackageSourcePyPI:
			default:
				return types.NewErrBadRequest("enforcement allowlist server entry %d has invalid package source %q (must be npm or pypi)", i, server.Package.Source)
			}
			if strings.TrimSpace(server.Package.Name) == "" {
				return types.NewErrBadRequest("enforcement allowlist server entry %d package requires a name", i)
			}
		}
	}
	return nil
}

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

// enforcementAllowlistForSave validates the incoming allowlist that will be
// stored on the configuration. When enforcement is enabled on a newly created
// configuration (current == nil) with no meaningful allowlist, the default is applied.
func enforcementAllowlistForSave(in types.MDMConfiguration, current *gtypes.MDMConfiguration) (types.EnforcementAllowlist, error) {
	allowlist := in.EnforcementAllowlist
	if enforcementAllowlistIsEmpty(allowlist) {
		if in.EnforcementEnabled && current == nil {
			return defaultEnforcementAllowlist(), nil
		}
		return types.EnforcementAllowlist{}, nil
	}
	if err := validateEnforcementAllowlist(allowlist); err != nil {
		return types.EnforcementAllowlist{}, err
	}
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

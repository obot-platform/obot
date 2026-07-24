package handlers

import (
	"net/url"
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
			if err := validateAllowlistURL(i, server.URL); err != nil {
				return err
			}
		}
		if server.Package != nil {
			set++
		}
		if strings.TrimSpace(server.Hostname) != "" {
			set++
			if err := validateAllowlistHostname(i, server.Hostname); err != nil {
				return err
			}
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

func validateAllowlistURL(index int, raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return types.NewErrBadRequest("enforcement allowlist server entry %d has an unparseable url %q: %v", index, raw, err)
	}
	switch {
	case u.Scheme != "http" && u.Scheme != "https":
		return types.NewErrBadRequest("enforcement allowlist server entry %d url %q must use the http or https scheme", index, raw)
	case u.Hostname() == "":
		return types.NewErrBadRequest("enforcement allowlist server entry %d url %q must include a hostname", index, raw)
	case u.User != nil:
		return types.NewErrBadRequest("enforcement allowlist server entry %d url %q must not include userinfo", index, raw)
	case u.RawQuery != "" || u.ForceQuery || u.Fragment != "":
		return types.NewErrBadRequest("enforcement allowlist server entry %d url %q must not include a query string or fragment; entries match on scheme, host, port, and path prefix", index, raw)
	}
	return nil
}

func validateAllowlistHostname(index int, raw string) error {
	if strings.ContainsAny(raw, ":/?#@ \t") {
		return types.NewErrBadRequest(
			"enforcement allowlist server entry %d hostname %q must be a bare hostname with no scheme, port, or path", index, raw)
	}
	return nil
}

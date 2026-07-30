package enforcement

import (
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
)

// CanonicalPackageName reduces a package name to the single spelling its
// registry considers canonical, so that packageMatches — which compares names
// as exact strings — cannot be sidestepped by an alias form.
func CanonicalPackageName(source types.AllowlistServerPackageSource, name string) string {
	name = strings.TrimSpace(name)
	switch source {
	case types.AllowlistServerPackageSourceNPM:
		// npm names are lowercase; the scope is part of the name and is
		// lowercased with it.
		return strings.ToLower(name)
	case types.AllowlistServerPackageSourcePyPI:
		return canonicalPyPIName(name)
	default:
		return name
	}
}

// canonicalPyPIName applies PEP 503 normalization — re.sub(`[-_.]+`, "-", name).lower()
// — so "Mcp_Server.Git" becomes "mcp-server-git" and "awslabs.core-mcp-server"
// becomes "awslabs-core-mcp-server", which is the project PyPI itself resolves.
//
// A leading or trailing separator run collapses to a single "-" and is kept, as
// the PEP's regex does. No legal package name has one, so this only matters for
// staying byte-identical to the device-side implementation on junk input.
func canonicalPyPIName(name string) string {
	var (
		out       strings.Builder
		separator bool
	)
	out.Grow(len(name))
	for _, r := range strings.ToLower(name) {
		if r == '-' || r == '_' || r == '.' {
			separator = true
			continue
		}
		if separator {
			out.WriteByte('-')
			separator = false
		}
		out.WriteRune(r)
	}
	if separator {
		out.WriteByte('-')
	}
	return out.String()
}

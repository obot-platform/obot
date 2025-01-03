package handlers

import (
	"os"
	"runtime/debug"
	"strings"

	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/version"
	"golang.org/x/mod/module"
	"sigs.k8s.io/yaml"
)

type VersionHandler struct {
	gptscriptVersion string
	emailDomain      string
	supportDocker    bool
}

func NewVersionHandler(emailDomain string, supportDocker bool) *VersionHandler {
	return &VersionHandler{
		emailDomain:      emailDomain,
		gptscriptVersion: getGPTScriptVersion(),
		supportDocker:    supportDocker,
	}
}

func (v *VersionHandler) GetVersion(req api.Context) error {
	return req.Write(v.getVersionResponse())
}

func (v *VersionHandler) getVersionResponse() map[string]any {
	values := make(map[string]any)
	versions := os.Getenv("OBOT_SERVER_VERSIONS")
	if versions != "" {
		if err := yaml.Unmarshal([]byte(versions), &values); err != nil {
			values["error"] = err.Error()
		}
	}
	values["obot"] = version.Get().String()
	values["gptscript"] = v.gptscriptVersion
	values["emailDomain"] = v.emailDomain
	values["dockerSupported"] = v.supportDocker
	return values
}

const gptscriptModulePath = "github.com/gptscript-ai/gptscript"

func getGPTScriptVersion() string {
	bi, _ := debug.ReadBuildInfo()

	var gptscriptVersion string
	for _, dep := range bi.Deps {
		if dep.Path == gptscriptModulePath {
			gptscriptVersion = simplifyModuleVersion(dep.Version)
			break
		}
	}

	return gptscriptVersion
}

// simplifyModuleVersion returns a simplified variant of a given module version string.
// If the given version is a Go pseudo-version, it strips the timestamp and truncates the revision to the first 7 characters.
// Empty strings and non-Go pseudo-versions are returned unaltered.
func simplifyModuleVersion(version string) string {
	if version == "" || !module.IsPseudoVersion(version) {
		return version
	}

	// Extract the base version (tag) and revision (commit hash)
	// Ignore errors, this should never happen if compilation succeeded
	components := make([]string, 0, 2)
	if base, err := module.PseudoVersionBase(version); err == nil && base != "" {
		components = append(components, base)
	}

	if rev, err := module.PseudoVersionRev(version); err == nil && len(rev) > 0 {
		// Shorten the hash to the first 7 characters
		if len(rev) > 7 {
			rev = rev[:7]
		}

		components = append(components, rev)
	}

	// Combine the base version with the shortened hash
	return strings.Join(components, "-")
}

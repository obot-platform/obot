package handlers

import (
	"os"
	"runtime/debug"
	"strings"

	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/version"
	"golang.org/x/mod/module"
)

type SessionStore string

const (
	SessionStoreDB     SessionStore = "db"
	SessionStoreCookie SessionStore = "cookie"
)

func sessionStoreFromPostgresDSN(postgresDSN string) SessionStore {
	if postgresDSN != "" {
		return SessionStoreDB
	}
	return SessionStoreCookie
}

type VersionHandler struct {
	gptscriptVersion string
	emailDomain      string
	supportDocker    bool
	authEnabled      bool
	sessionStore     SessionStore
}

func NewVersionHandler(emailDomain, postgresDSN string, supportDocker, authEnabled bool) *VersionHandler {
	return &VersionHandler{
		emailDomain:      emailDomain,
		gptscriptVersion: getGPTScriptVersion(),
		supportDocker:    supportDocker,
		authEnabled:      authEnabled,
		sessionStore:     sessionStoreFromPostgresDSN(postgresDSN),
	}
}

func (v *VersionHandler) GetVersion(req api.Context) error {
	return req.Write(v.getVersionResponse())
}

func (v *VersionHandler) getVersionResponse() map[string]any {
	values := make(map[string]any)
	versions := os.Getenv("OBOT_SERVER_VERSIONS")
	if versions != "" {
		pairs := strings.Split(versions, ",")
		for _, pair := range pairs {
			if pair == "" {
				continue
			}
			parts := strings.SplitN(pair, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			values[key] = value
		}
	}
	values["obot"] = version.Get().String()
	values["gptscript"] = v.gptscriptVersion
	values["emailDomain"] = v.emailDomain
	values["dockerSupported"] = v.supportDocker
	values["authEnabled"] = v.authEnabled
	values["sessionStore"] = v.sessionStore
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

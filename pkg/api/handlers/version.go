package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/version"
	"golang.org/x/mod/module"
	"gorm.io/gorm"
)

type SessionStore string

const (
	SessionStoreDB     SessionStore = "db"
	SessionStoreCookie SessionStore = "cookie"

	installationIDPropertyKey   = "installation_id"
	defaultUpgradeServerBaseURL = "https://upgrade-server.obot.ai"
	updateCheckInterval         = 24 * time.Hour
)

func sessionStoreFromPostgresDSN(postgresDSN string) SessionStore {
	if postgresDSN != "" {
		return SessionStoreDB
	}
	return SessionStoreCookie
}

type VersionHandler struct {
	gptscriptVersion         string
	emailDomain              string
	supportDocker            bool
	authEnabled              bool
	sessionStore             SessionStore
	enterprise               bool
	engine                   string
	autonomousToolUseEnabled bool

	upgradeServerURL string
	upgradeAvailable bool
	latestVersion    string
	upgradeLock      sync.RWMutex
}

func NewVersionHandler(ctx context.Context, gatewayClient *client.Client, emailDomain, postgresDSN, engine string, supportDocker, authEnabled, disableUpdateCheck, autonomousToolUseEnabled bool) (*VersionHandler, error) {
	upgradeServerBaseURL := defaultUpgradeServerBaseURL
	if os.Getenv("OBOT_UPGRADE_SERVER_URL") != "" {
		upgradeServerBaseURL = os.Getenv("OBOT_UPGRADE_SERVER_URL")
	}

	v := &VersionHandler{
		emailDomain:              emailDomain,
		gptscriptVersion:         getGPTScriptVersion(),
		supportDocker:            supportDocker,
		authEnabled:              authEnabled,
		sessionStore:             sessionStoreFromPostgresDSN(postgresDSN),
		enterprise:               os.Getenv("OBOT_ENTERPRISE") == "true",
		upgradeServerURL:         fmt.Sprintf("%s/check-upgrade", upgradeServerBaseURL),
		engine:                   engine,
		autonomousToolUseEnabled: autonomousToolUseEnabled,
	}

	currentVersion, _, _ := strings.Cut(version.Get().String(), "+")
	currentVersion, _, _ = strings.Cut(currentVersion, "-")

	// Don't start the upgrade check if the interval is non-positive or if this is a development version.
	if !disableUpdateCheck && (!strings.HasPrefix(currentVersion, "v0.0.0") || os.Getenv("OBOT_FORCE_UPGRADE_CHECK") == "true") {
		p, err := gatewayClient.GetProperty(ctx, installationIDPropertyKey)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			p, err = gatewayClient.SetProperty(ctx, installationIDPropertyKey, uuid.NewString())
			if err != nil {
				return nil, fmt.Errorf("failed to set installation ID property: %w", err)
			}
		} else if err != nil {
			return nil, fmt.Errorf("failed to get installation ID property: %w", err)
		}

		distribution := "oss"
		if v.enterprise {
			distribution = "enterprise"
		}

		go v.startUpgradeCheck(ctx, p.Value, currentVersion, engine, distribution)
	}

	return v, nil
}

func (v *VersionHandler) GetVersion(req api.Context) error {
	return req.Write(v.getVersionResponse())
}

func (v *VersionHandler) getVersionResponse() map[string]any {
	versions := os.Getenv("OBOT_SERVER_VERSIONS")
	values := make(map[string]any, len(versions)+9)
	if versions != "" {
		for pair := range strings.SplitSeq(versions, ",") {
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
	values["enterprise"] = v.enterprise
	values["engine"] = v.engine
	values["autonomousToolUseEnabled"] = v.autonomousToolUseEnabled
	v.upgradeLock.RLock()
	values["upgradeAvailable"] = v.upgradeAvailable
	values["latestVersion"] = v.latestVersion
	v.upgradeLock.RUnlock()

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

func (v *VersionHandler) startUpgradeCheck(ctx context.Context, installationID, currentVersion, engine, distribution string) {
	timer := time.NewTimer(updateCheckInterval)
	defer timer.Stop()

	var err error
	for {
		if err = v.checkForUpgrade(ctx, installationID, currentVersion, engine, distribution); err != nil {
			log.Debugf("failed to check for server upgrade: %v", err)
		}

		select {
		case <-ctx.Done():
			log.Debugf("upgrade check context cancelled, exiting")
			return
		case <-timer.C:
			timer.Reset(updateCheckInterval)
		}
	}
}

func (v *VersionHandler) checkForUpgrade(ctx context.Context, installationID, currentVersion, engine, distribution string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.upgradeServerURL, nil)
	if err != nil {
		return err
	}

	query := req.URL.Query()
	query.Set("uid", installationID)
	query.Set("engine", engine)
	query.Set("distribution", distribution)
	query.Set("current-version", currentVersion)
	req.URL.RawQuery = query.Encode()

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	var upgradeInfo upgradeCheckResponse
	if err := json.NewDecoder(resp.Body).Decode(&upgradeInfo); err != nil {
		return err
	}

	v.upgradeLock.RLock()
	currentUpgradeAvailable := v.upgradeAvailable
	latestVersion := v.latestVersion
	v.upgradeLock.RUnlock()

	if currentUpgradeAvailable != upgradeInfo.UpgradeAvailable || latestVersion != upgradeInfo.LatestVersion {
		v.upgradeLock.Lock()
		v.upgradeAvailable = upgradeInfo.UpgradeAvailable
		v.latestVersion = upgradeInfo.LatestVersion
		v.upgradeLock.Unlock()
	}

	return nil
}

type upgradeCheckResponse struct {
	UpgradeAvailable bool   `json:"upgradeAvailable"`
	LatestVersion    string `json:"latestVersion"`
	CurrentVersion   string `json:"currentVersion"`
}

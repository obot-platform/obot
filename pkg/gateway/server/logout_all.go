package server

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/obot-platform/obot/pkg/api"
	gcontext "github.com/obot-platform/obot/pkg/gateway/context"
	"github.com/obot-platform/obot/pkg/proxy"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/tidwall/gjson"
)

func (s *Server) logoutAll(apiContext api.Context) error {
	logger := gcontext.GetLogger(apiContext.Context())

	sessionID, sessionIDFound := getSessionID(apiContext.Request)

	identities, err := s.client.FindIdentitiesForUser(apiContext.Context(), apiContext.UserID())
	if err != nil {
		return err
	}

	var errs []error
	for _, identity := range identities {
		if identity.AuthProviderName == "" || identity.AuthProviderNamespace == "" {
			continue
		}

		var ref v1.ToolReference
		if err := apiContext.Get(&ref, identity.AuthProviderName); err != nil {
			errs = append(errs, fmt.Errorf("failed to get auth provider %q: %w", identity.AuthProviderName, err))
			continue
		}

		user := identity.ProviderUserID
		if identity.AuthProviderName == "github-auth-provider" && identity.AuthProviderNamespace == system.DefaultNamespace {
			// The GitHub auth provider stores the username as the user ID in the sessions table.
			// This is because of an annoying quirk of the oauth2-proxy code for GitHub,
			// where we do not know the real user ID until after the user has logged in and the session is created,
			// and we have to manually fetch it from the GitHub API.
			// The oauth2-proxy is only aware of the username, which is why that's in the sessions table.
			user = identity.ProviderUsername
		}

		emailHash := sha256.Sum256([]byte(identity.Email))
		userHash := sha256.Sum256([]byte(user))

		logger.Debug("deleting sessions for provider", "provider", identity.AuthProviderName)

		if meta, ok := ref.Status.Tool.Metadata["providerMeta"]; ok {
			tablePrefix := gjson.Get(meta, "postgresTablePrefix").String()
			if tablePrefix != "" {
				var err error
				if sessionIDFound {
					err = s.db.WithContext(apiContext.Context()).Exec("DELETE FROM "+tablePrefix+"sessions WHERE key NOT LIKE ? AND \"user\" = decode(?, 'hex') AND \"email\" = decode(?, 'hex')", sessionID+"%", fmt.Sprintf("%x", userHash), fmt.Sprintf("%x", emailHash)).Error
				} else {
					err = s.db.WithContext(apiContext.Context()).Exec("DELETE FROM "+tablePrefix+"sessions WHERE \"user\" = decode(?, 'hex') AND \"email\" = decode(?, 'hex')", fmt.Sprintf("%x", userHash), fmt.Sprintf("%x", emailHash)).Error
				}

				if err != nil {
					errs = append(errs, fmt.Errorf("failed to delete sessions for provider %q: %w", identity.AuthProviderName, err))
				} else {
					logger.Debug("deleted sessions for provider", "provider", identity.AuthProviderName)
				}
			}
		}
	}

	return errors.Join(errs...)
}

func getSessionID(req *http.Request) (string, bool) {
	cookie, err := req.Cookie(proxy.ObotAccessTokenCookie)
	if err != nil {
		return "", false
	}

	// If the cookie is an oauth2-proxy ticket cookie, it should be three segments separated by pipes.
	// The first one contains the session ID.
	parts := strings.Split(cookie.Value, "|")
	if len(parts) != 3 {
		return "", false
	}

	firstPart, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		return "", false
	}

	// This first part, after decoding, is three parts, separated by dots.
	// The middle one is the session ID encoded in base64.
	parts = strings.Split(string(firstPart), ".")
	if len(parts) != 3 {
		return "", false
	}

	// Strangely, the session ID is usually not quite complete.
	// I think it gets truncated at some point. So we have to ignore errors when decoding.
	// We will still get part of the decoded session ID, and it's a long enough prefix to work.
	decodedID, _ := base64.StdEncoding.DecodeString(parts[1])
	// If it's not at least 10 characters, we can't really use it.
	// I've never seen this happen in testing, but it's best to be safe.
	if len(decodedID) < 10 {
		return "", false
	}

	return string(decodedID), true
}

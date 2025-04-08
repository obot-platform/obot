package server

import (
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/proxy"
)

func (s *Server) logoutAll(apiContext api.Context) error {
	sessionID := getSessionID(apiContext.Request)

	identities, err := s.client.FindIdentitiesForUser(apiContext.Context(), apiContext.UserID())
	if err != nil {
		return err
	}

	return s.client.DeleteSessionsForUser(apiContext.Context(), s.storageClient, identities, sessionID)
}

func getSessionID(req *http.Request) string {
	cookie, err := req.Cookie(proxy.ObotAccessTokenCookie)
	if err != nil {
		return ""
	}

	// If the cookie is an oauth2-proxy ticket cookie, it should be three segments separated by pipes.
	// The first one contains the session ID.
	parts := strings.Split(cookie.Value, "|")
	if len(parts) != 3 {
		return ""
	}

	firstPart, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		return ""
	}

	// This first part, after decoding, is three parts, separated by dots.
	// The middle one is the session ID encoded in base64.
	parts = strings.Split(string(firstPart), ".")
	if len(parts) != 3 {
		return ""
	}

	// Strangely, the session ID is usually not quite complete.
	// I think it gets truncated at some point. So we have to ignore errors when decoding.
	// We will still get part of the decoded session ID, and it's a long enough prefix to work.
	decodedID, _ := base64.StdEncoding.DecodeString(parts[1])
	// If it's not at least 10 characters, we can't really use it.
	// I've never seen this happen in testing, but it's best to be safe.
	if len(decodedID) < 10 {
		return ""
	}

	return string(decodedID)
}

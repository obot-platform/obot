package server

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"gorm.io/gorm"
)

type createAPIKeyRequest struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
}

// createAPIKey creates an API key for the authenticated user.
func (s *Server) createAPIKey(apiContext api.Context) error {
	var req createAPIKeyRequest
	if err := apiContext.Read(&req); err != nil {
		return types2.NewErrBadRequest("invalid request body: %v", err)
	}

	if req.Name == "" {
		return types2.NewErrBadRequest("name is required")
	}

	userID := apiContext.UserID()
	if userID == 0 {
		return types2.NewErrHTTP(http.StatusUnauthorized, "user not authenticated")
	}

	response, err := apiContext.GatewayClient.CreateAPIKey(apiContext.Context(), userID, req.Name, req.Description, req.ExpiresAt)
	if err != nil {
		return types2.NewErrHTTP(http.StatusInternalServerError, fmt.Sprintf("failed to create API key: %v", err))
	}

	return apiContext.WriteCreated(response)
}

// listAPIKeys lists all API keys for the authenticated user.
func (s *Server) listAPIKeys(apiContext api.Context) error {
	userID := apiContext.UserID()
	if userID == 0 {
		return types2.NewErrHTTP(http.StatusUnauthorized, "user not authenticated")
	}

	keys, err := apiContext.GatewayClient.ListAPIKeys(apiContext.Context(), userID)
	if err != nil {
		return types2.NewErrHTTP(http.StatusInternalServerError, fmt.Sprintf("failed to list API keys: %v", err))
	}

	return apiContext.Write(map[string]any{"items": keys})
}

// getAPIKey gets a single API key for the authenticated user.
func (s *Server) getAPIKey(apiContext api.Context) error {
	userID := apiContext.UserID()
	if userID == 0 {
		return types2.NewErrHTTP(http.StatusUnauthorized, "user not authenticated")
	}

	keyID, err := strconv.ParseUint(apiContext.PathValue("id"), 10, 64)
	if err != nil {
		return types2.NewErrBadRequest("invalid key ID")
	}

	key, err := apiContext.GatewayClient.GetAPIKey(apiContext.Context(), userID, uint(keyID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types2.NewErrNotFound("API key not found")
		}
		return types2.NewErrHTTP(http.StatusInternalServerError, fmt.Sprintf("failed to get API key: %v", err))
	}

	return apiContext.Write(key)
}

// deleteAPIKey deletes an API key for the authenticated user.
func (s *Server) deleteAPIKey(apiContext api.Context) error {
	userID := apiContext.UserID()
	if userID == 0 {
		return types2.NewErrHTTP(http.StatusUnauthorized, "user not authenticated")
	}

	keyID, err := strconv.ParseUint(apiContext.PathValue("id"), 10, 64)
	if err != nil {
		return types2.NewErrBadRequest("invalid key ID")
	}

	if err := apiContext.GatewayClient.DeleteAPIKey(apiContext.Context(), userID, uint(keyID)); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types2.NewErrNotFound("API key not found")
		}
		return types2.NewErrHTTP(http.StatusInternalServerError, fmt.Sprintf("failed to delete API key: %v", err))
	}

	return apiContext.Write(map[string]any{"deleted": true})
}

// Admin endpoints for managing any user's API keys

// listAllAPIKeys lists all API keys in the system (admin/owner only).
func (s *Server) listAllAPIKeys(apiContext api.Context) error {
	keys, err := apiContext.GatewayClient.ListAllAPIKeys(apiContext.Context())
	if err != nil {
		return types2.NewErrHTTP(http.StatusInternalServerError, fmt.Sprintf("failed to list API keys: %v", err))
	}

	return apiContext.Write(map[string]any{"items": keys})
}

// getAnyAPIKey gets any API key by ID (admin/owner only).
func (s *Server) getAnyAPIKey(apiContext api.Context) error {
	keyID, err := strconv.ParseUint(apiContext.PathValue("id"), 10, 64)
	if err != nil {
		return types2.NewErrBadRequest("invalid key ID")
	}

	key, err := apiContext.GatewayClient.GetAPIKeyByID(apiContext.Context(), uint(keyID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types2.NewErrNotFound("API key not found")
		}
		return types2.NewErrHTTP(http.StatusInternalServerError, fmt.Sprintf("failed to get API key: %v", err))
	}

	return apiContext.Write(key)
}

// deleteAnyAPIKey deletes any API key by ID (admin/owner only).
func (s *Server) deleteAnyAPIKey(apiContext api.Context) error {
	keyID, err := strconv.ParseUint(apiContext.PathValue("id"), 10, 64)
	if err != nil {
		return types2.NewErrBadRequest("invalid key ID")
	}

	if err := apiContext.GatewayClient.DeleteAPIKeyByID(apiContext.Context(), uint(keyID)); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types2.NewErrNotFound("API key not found")
		}
		return types2.NewErrHTTP(http.StatusInternalServerError, fmt.Sprintf("failed to delete API key: %v", err))
	}

	return apiContext.Write(map[string]any{"deleted": true})
}

// Authentication webhook endpoint

type apiKeyAuthRequest struct {
	MCPServerID         string `json:"mcpServerId,omitempty"`
	MCPServerInstanceID string `json:"mcpServerInstanceId,omitempty"`
}

type apiKeyAuthResponse struct {
	Authenticated bool   `json:"authenticated"`
	Authorized    bool   `json:"authorized"`
	UserID        uint   `json:"userId,omitempty"`
	Username      string `json:"username,omitempty"`
	Error         string `json:"error,omitempty"`
}

func (s *Server) authenticateAPIKey(apiContext api.Context) error {
	// Extract API key from header
	authHeader := apiContext.Request.Header.Get("Authorization")
	if authHeader == "" {
		return apiContext.Write(apiKeyAuthResponse{
			Authenticated: false,
			Error:         "missing Authorization header",
		})
	}

	bearer := strings.TrimPrefix(authHeader, "Bearer ")
	if bearer == authHeader || !strings.HasPrefix(bearer, "ok1-") {
		return apiContext.Write(apiKeyAuthResponse{
			Authenticated: false,
			Error:         "invalid API key format",
		})
	}

	// Parse request body for MCP server info (optional)
	var req apiKeyAuthRequest
	// Ignore read errors - body may be empty
	_ = apiContext.Read(&req)

	// Validate the API key
	apiKey, err := apiContext.GatewayClient.ValidateAPIKey(apiContext.Context(), bearer)
	if err != nil {
		return apiContext.Write(apiKeyAuthResponse{
			Authenticated: false,
			Error:         "invalid or expired API key",
		})
	}

	// Get user info
	user, err := apiContext.GatewayClient.UserByID(apiContext.Context(), strconv.FormatUint(uint64(apiKey.UserID), 10))
	if err != nil {
		return apiContext.Write(apiKeyAuthResponse{
			Authenticated: false,
			Error:         "user not found",
		})
	}

	// For now, authorization is always true if authentication succeeds
	// MCP server permission checks will be added in a later phase
	return apiContext.Write(apiKeyAuthResponse{
		Authenticated: true,
		Authorized:    true,
		UserID:        user.ID,
		Username:      user.Username,
	})
}

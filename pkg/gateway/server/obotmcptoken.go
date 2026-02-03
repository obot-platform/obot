package server

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"gorm.io/gorm"
)

// createObotMCPToken creates an MCP token for the authenticated user.
func (s *Server) createObotMCPToken(apiContext api.Context) error {
	userID := apiContext.UserID()
	if userID == 0 {
		return types2.NewErrHTTP(http.StatusUnauthorized, "user not authenticated")
	}

	response, err := apiContext.GatewayClient.CreateObotMCPToken(apiContext.Context(), userID)
	if err != nil {
		return types2.NewErrHTTP(http.StatusInternalServerError, fmt.Sprintf("failed to create MCP token: %v", err))
	}

	return apiContext.WriteCreated(response)
}

// listObotMCPTokens lists all MCP tokens for the authenticated user.
func (s *Server) listObotMCPTokens(apiContext api.Context) error {
	userID := apiContext.UserID()
	if userID == 0 {
		return types2.NewErrHTTP(http.StatusUnauthorized, "user not authenticated")
	}

	tokens, err := apiContext.GatewayClient.ListObotMCPTokens(apiContext.Context(), userID)
	if err != nil {
		return types2.NewErrHTTP(http.StatusInternalServerError, fmt.Sprintf("failed to list MCP tokens: %v", err))
	}

	return apiContext.Write(map[string]any{"items": tokens})
}

// getObotMCPToken gets a single MCP token for the authenticated user.
func (s *Server) getObotMCPToken(apiContext api.Context) error {
	userID := apiContext.UserID()
	if userID == 0 {
		return types2.NewErrHTTP(http.StatusUnauthorized, "user not authenticated")
	}

	tokenID, err := strconv.ParseUint(apiContext.PathValue("id"), 10, 64)
	if err != nil {
		return types2.NewErrBadRequest("invalid token ID")
	}

	token, err := apiContext.GatewayClient.GetObotMCPToken(apiContext.Context(), userID, uint(tokenID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types2.NewErrNotFound("MCP token not found")
		}
		return types2.NewErrHTTP(http.StatusInternalServerError, fmt.Sprintf("failed to get MCP token: %v", err))
	}

	return apiContext.Write(token)
}

// deleteObotMCPToken deletes an MCP token for the authenticated user.
func (s *Server) deleteObotMCPToken(apiContext api.Context) error {
	userID := apiContext.UserID()
	if userID == 0 {
		return types2.NewErrHTTP(http.StatusUnauthorized, "user not authenticated")
	}

	tokenID, err := strconv.ParseUint(apiContext.PathValue("id"), 10, 64)
	if err != nil {
		return types2.NewErrBadRequest("invalid token ID")
	}

	if err := apiContext.GatewayClient.DeleteObotMCPToken(apiContext.Context(), userID, uint(tokenID)); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types2.NewErrNotFound("MCP token not found")
		}
		return types2.NewErrHTTP(http.StatusInternalServerError, fmt.Sprintf("failed to delete MCP token: %v", err))
	}

	return apiContext.Write(map[string]any{"deleted": true})
}

package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/subjectgroups"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// CreateToolListRequest starts a request, usually from the obot CLI, to list a catalog entry's
// tools. Nothing launches until the user confirms it on the entry's Tools tab in the Obot UI.
func (h *MCPCatalogHandler) CreateToolListRequest(req api.Context) error {
	if !subjectgroups.IsAdmin(req.User) {
		return types.NewErrForbidden("only admins and owners can list catalog entries' tools")
	}
	var manifest types.ToolListRequestManifest
	if err := req.Read(&manifest); err != nil {
		return types.NewErrBadRequest("failed to read tool list request: %v", err)
	}
	entry, err := toolListEntry(req, manifest.EntryID)
	if err != nil {
		return err
	}
	request, err := req.GatewayClient.CreateToolListRequest(req.Context(), req.User.GetUID(), entry.Name)
	if err != nil {
		return err
	}
	return req.WriteCreated(h.convertToolListRequest(*request, entry))
}

// GetToolListRequest returns one of the caller's tool list requests, which the CLI polls.
func (h *MCPCatalogHandler) GetToolListRequest(req api.Context) error {
	request, err := callersToolListRequest(req)
	if err != nil {
		return err
	}
	entry, err := toolListEntry(req, request.EntryName)
	if err != nil {
		return err
	}
	return req.Write(h.convertToolListRequest(*request, entry))
}

// CompleteToolListRequest records the tools the user listed on the entry's Tools tab, where they
// confirmed launching the entry's server, or why listing them failed, so the CLI gets the result.
// Only the user who made the request can complete it, from a browser: an API key, which an agent
// could hold, can't.
func (h *MCPCatalogHandler) CompleteToolListRequest(req api.Context) error {
	request, err := browserToolListRequest(req)
	if err != nil {
		return err
	}
	var result struct {
		Tools []types.MCPServerTool `json:"tools"`
		// Error is why the tools couldn't be listed.
		Error string `json:"error"`
	}
	if err := req.Read(&result); err != nil {
		return types.NewErrBadRequest("failed to read the listed tools: %v", err)
	}
	var listErr error
	if result.Error != "" {
		listErr = errors.New(result.Error)
	}
	if err := req.GatewayClient.CompleteToolListRequest(req.Context(), request.UserID, request.ID, result.Tools, listErr); err != nil {
		return toolListRequestError(err)
	}
	return nil
}

// CancelToolListRequest records that the user declined to launch the entry's server, so the CLI stops waiting.
func (h *MCPCatalogHandler) CancelToolListRequest(req api.Context) error {
	request, err := browserToolListRequest(req)
	if err != nil {
		return err
	}
	if err := req.GatewayClient.CompleteToolListRequest(req.Context(), request.UserID, request.ID, nil, errors.New("the user declined to launch the server")); err != nil {
		return toolListRequestError(err)
	}
	return nil
}

// callersToolListRequest returns the caller's tool list request named in the path.
func callersToolListRequest(req api.Context) (*gatewaytypes.ToolListRequest, error) {
	if !subjectgroups.IsAdmin(req.User) {
		return nil, types.NewErrForbidden("only admins and owners can list catalog entries' tools")
	}
	request, err := req.GatewayClient.ToolListRequest(req.Context(), req.User.GetUID(), req.PathValue("request_id"))
	if err != nil {
		return nil, toolListRequestError(err)
	}
	return request, nil
}

// browserToolListRequest returns the caller's tool list request named in the path, if the caller
// is a person in a browser session rather than an API key.
func browserToolListRequest(req api.Context) (*gatewaytypes.ToolListRequest, error) {
	if _, ok := principal.APIKeyAttributionFromUser(req.User); ok {
		return nil, types.NewErrForbidden("tool list requests are confirmed in the Obot UI, not with an API key")
	}
	return callersToolListRequest(req)
}

// toolListEntry returns the catalog entry with id, if it's an entry of a catalog.
func toolListEntry(req api.Context, id string) (v1.MCPServerCatalogEntry, error) {
	var entry v1.MCPServerCatalogEntry
	if id == "" {
		return entry, types.NewErrBadRequest("entryID is required")
	}
	if err := req.Get(&entry, id); apierrors.IsNotFound(err) || err == nil && entry.Spec.MCPCatalogName == "" {
		return entry, types.NewErrNotFound("catalog entry %q not found", id)
	} else if err != nil {
		return entry, fmt.Errorf("failed to get catalog entry: %w", err)
	}
	return entry, nil
}

func toolListRequestError(err error) error {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return types.NewErrNotFound("tool list request not found; it may have expired")
	case errors.Is(err, gclient.ErrToolListRequestNotPending):
		return types.NewErrHTTP(http.StatusConflict, "this tool list request was already answered")
	}
	return err
}

func (h *MCPCatalogHandler) convertToolListRequest(request gatewaytypes.ToolListRequest, entry v1.MCPServerCatalogEntry) types.ToolListRequest {
	return types.ToolListRequest{
		ID:        request.ID,
		EntryID:   entry.Name,
		EntryName: entry.Spec.Manifest.Name,
		EntryIcon: entry.Spec.Manifest.Icon,
		CatalogID: entry.Spec.MCPCatalogName,
		Status:    request.Status,
		Tools:     request.Tools,
		Error:     request.Error,
		ExpiresAt: types.NewTime(request.ExpiresAt),
		URL:       h.serverURL + "/mcp-servers/c/" + url.PathEscape(entry.Name) + "?view=tools&toolListRequest=" + url.QueryEscape(request.ID),
	}
}

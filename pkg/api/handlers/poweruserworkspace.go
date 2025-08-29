package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type PowerUserWorkspaceResponse struct {
	ID               string                                    `json:"id"`
	UserID           string                                   `json:"userID"`
	Role             types2.Role                              `json:"role"`
	Created          metav1.Time                              `json:"created"`
	ResourceCounts   v1.PowerUserWorkspaceResourceCounts     `json:"resourceCounts"`
}

func ListPowerUserWorkspaces(req api.Context) error {
	userID := fmt.Sprintf("%d", req.User.GetUID())

	// Only allow users to see their own workspace, unless they're admin
	isAdmin := req.UserIsAdmin()
	
	workspaceList := &v1.PowerUserWorkspaceList{}
	
	if isAdmin {
		// Admins can see all workspaces
		if err := req.Storage.List(req.Context(), workspaceList, kclient.InNamespace(system.DefaultNamespace)); err != nil {
			return err
		}
	} else {
		// Non-admins can only see their own workspace
		workspaceName := fmt.Sprintf("workspace-%s", userID)
		workspace := &v1.PowerUserWorkspace{}
		err := req.Storage.Get(req.Context(), kclient.ObjectKey{
			Namespace: system.DefaultNamespace,
			Name:      workspaceName,
		}, workspace)
		
		if apierrors.IsNotFound(err) {
			// User doesn't have a workspace, return empty list
			return req.Write([]PowerUserWorkspaceResponse{})
		} else if err != nil {
			return err
		}
		
		workspaceList.Items = []v1.PowerUserWorkspace{*workspace}
	}

	responses := make([]PowerUserWorkspaceResponse, len(workspaceList.Items))
	for i, workspace := range workspaceList.Items {
		responses[i] = PowerUserWorkspaceResponse{
			ID:             workspace.Name,
			UserID:         workspace.Spec.UserID,
			Role:           workspace.Spec.Role,
			Created:        workspace.CreationTimestamp,
			ResourceCounts: workspace.Status.ResourceCounts,
		}
	}

	return req.Write(responses)
}

func GetPowerUserWorkspace(req api.Context) error {
	workspaceID := req.PathValue("id")
	userID := fmt.Sprintf("%d", req.User.GetUID())
	
	// Check if user can access this workspace
	if !canAccessWorkspace(req, workspaceID, userID) {
		return types2.NewErrHTTP(http.StatusForbidden, "Access denied to workspace")
	}

	workspace := &v1.PowerUserWorkspace{}
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      workspaceID,
	}, workspace); err != nil {
		if apierrors.IsNotFound(err) {
			return types2.NewErrHTTP(http.StatusNotFound, "Workspace not found")
		}
		return err
	}

	response := PowerUserWorkspaceResponse{
		ID:             workspace.Name,
		UserID:         workspace.Spec.UserID,
		Role:           workspace.Spec.Role,
		Created:        workspace.CreationTimestamp,
		ResourceCounts: workspace.Status.ResourceCounts,
	}

	return req.Write(response)
}

func ListWorkspaceMCPServers(req api.Context) error {
	workspaceID := req.PathValue("id")
	userID := fmt.Sprintf("%d", req.User.GetUID())
	
	if !canAccessWorkspace(req, workspaceID, userID) {
		return types2.NewErrHTTP(http.StatusForbidden, "Access denied to workspace")
	}

	mcpServerList := &v1.MCPServerList{}
	if err := req.Storage.List(req.Context(), mcpServerList,
		kclient.InNamespace(system.DefaultNamespace),
		kclient.MatchingFields{"spec.powerUserWorkspaceID": workspaceID}); err != nil {
		return err
	}

	// Convert to API response format
	servers := make([]types2.MCPServer, len(mcpServerList.Items))
	for i, server := range mcpServerList.Items {
		servers[i] = types2.MCPServer{
			Metadata: types2.Metadata{
				ID:      server.Name,
				Created: *types2.NewTime(server.CreationTimestamp.Time),
			},
			MCPServerManifest: server.Spec.Manifest,
		}
	}

	return req.Write(servers)
}

func CreateWorkspaceMCPServer(req api.Context) error {
	workspaceID := req.PathValue("id")
	userID := fmt.Sprintf("%d", req.User.GetUID())
	
	if !canAccessWorkspace(req, workspaceID, userID) {
		return types2.NewErrHTTP(http.StatusForbidden, "Access denied to workspace")
	}

	// Verify workspace exists and user has permission to create multi-user servers
	workspace := &v1.PowerUserWorkspace{}
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      workspaceID,
	}, workspace); err != nil {
		if apierrors.IsNotFound(err) {
			return types2.NewErrHTTP(http.StatusNotFound, "Workspace not found")
		}
		return err
	}

	// Only PowerUserPlus and Admin can create multi-user MCPServers
	if workspace.Spec.Role != types2.RolePowerUserPlus && workspace.Spec.Role != types2.RoleAdmin {
		return types2.NewErrHTTP(http.StatusForbidden, "Only PowerUser Plus and Admin roles can create multi-user MCP servers")
	}

	var mcpServerReq types2.MCPServer
	if err := json.NewDecoder(req.Request.Body).Decode(&mcpServerReq); err != nil {
		return types2.NewErrBadRequest("Invalid JSON: %v", err)
	}

	// Create MCPServer with workspace reference
	mcpServer := &v1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    system.DefaultNamespace,
			GenerateName: "workspace-mcpserver-",
		},
		Spec: v1.MCPServerSpec{
			Manifest:              mcpServerReq.MCPServerManifest,
			PowerUserWorkspaceID:  workspaceID,
			UserID:                userID,
		},
	}

	if err := req.Storage.Create(req.Context(), mcpServer); err != nil {
		return err
	}

	response := types2.MCPServer{
		Metadata: types2.Metadata{
			ID:      mcpServer.Name,
			Created: *types2.NewTime(mcpServer.CreationTimestamp.Time),
		},
		MCPServerManifest: mcpServer.Spec.Manifest,
	}

	return req.Write(response)
}

func ListWorkspaceMCPServerCatalogEntries(req api.Context) error {
	workspaceID := req.PathValue("id")
	userID := fmt.Sprintf("%d", req.User.GetUID())
	
	if !canAccessWorkspace(req, workspaceID, userID) {
		return types2.NewErrHTTP(http.StatusForbidden, "Access denied to workspace")
	}

	entryList := &v1.MCPServerCatalogEntryList{}
	if err := req.Storage.List(req.Context(), entryList,
		kclient.InNamespace(system.DefaultNamespace),
		kclient.MatchingFields{"spec.powerUserWorkspaceID": workspaceID}); err != nil {
		return err
	}

	// Convert to API response format
	entries := make([]types2.MCPServerCatalogEntry, len(entryList.Items))
	for i, entry := range entryList.Items {
		entries[i] = types2.MCPServerCatalogEntry{
			Metadata: types2.Metadata{
				ID:      entry.Name,
				Created: *types2.NewTime(entry.CreationTimestamp.Time),
			},
			Manifest: entry.Spec.Manifest,
		}
	}

	return req.Write(entries)
}

func CreateWorkspaceMCPServerCatalogEntry(req api.Context) error {
	workspaceID := req.PathValue("id")
	userID := fmt.Sprintf("%d", req.User.GetUID())
	
	if !canAccessWorkspace(req, workspaceID, userID) {
		return types2.NewErrHTTP(http.StatusForbidden, "Access denied to workspace")
	}

	var entryReq types2.MCPServerCatalogEntry
	if err := json.NewDecoder(req.Request.Body).Decode(&entryReq); err != nil {
		return types2.NewErrBadRequest("Invalid JSON: %v", err)
	}

	// Create MCPServerCatalogEntry with workspace reference
	entry := &v1.MCPServerCatalogEntry{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    system.DefaultNamespace,
			GenerateName: "workspace-entry-",
		},
		Spec: v1.MCPServerCatalogEntrySpec{
			Manifest:              entryReq.Manifest,
			PowerUserWorkspaceID:  workspaceID,
			MCPCatalogName:        system.DefaultCatalog, // Power users create entries in default catalog
			Editable:              true,
		},
	}

	if err := req.Storage.Create(req.Context(), entry); err != nil {
		return err
	}

	response := types2.MCPServerCatalogEntry{
		Metadata: types2.Metadata{
			ID:      entry.Name,
			Created: *types2.NewTime(entry.CreationTimestamp.Time),
		},
		Manifest: entry.Spec.Manifest,
	}

	return req.Write(response)
}

func ListWorkspaceAccessControlRules(req api.Context) error {
	workspaceID := req.PathValue("id")
	userID := fmt.Sprintf("%d", req.User.GetUID())
	
	if !canAccessWorkspace(req, workspaceID, userID) {
		return types2.NewErrHTTP(http.StatusForbidden, "Access denied to workspace")
	}

	// Verify workspace exists and user has permission to manage ACRs
	workspace := &v1.PowerUserWorkspace{}
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      workspaceID,
	}, workspace); err != nil {
		if apierrors.IsNotFound(err) {
			return types2.NewErrHTTP(http.StatusNotFound, "Workspace not found")
		}
		return err
	}

	// Only PowerUserPlus and Admin can manage ACRs
	if workspace.Spec.Role != types2.RolePowerUserPlus && workspace.Spec.Role != types2.RoleAdmin {
		return types2.NewErrHTTP(http.StatusForbidden, "Only PowerUser Plus and Admin roles can manage Access Control Rules")
	}

	acrList := &v1.AccessControlRuleList{}
	if err := req.Storage.List(req.Context(), acrList,
		kclient.InNamespace(system.DefaultNamespace),
		kclient.MatchingFields{"spec.powerUserWorkspaceID": workspaceID}); err != nil {
		return err
	}

	// Convert to API response format
	rules := make([]types2.AccessControlRule, len(acrList.Items))
	for i, rule := range acrList.Items {
		rules[i] = types2.AccessControlRule{
			Metadata: types2.Metadata{
				ID:      rule.Name,
				Created: *types2.NewTime(rule.CreationTimestamp.Time),
			},
			MCPCatalogID:              rule.Spec.MCPCatalogID,
			AccessControlRuleManifest: rule.Spec.Manifest,
		}
	}

	return req.Write(rules)
}

func CreateWorkspaceAccessControlRule(req api.Context) error {
	workspaceID := req.PathValue("id")
	userID := fmt.Sprintf("%d", req.User.GetUID())
	
	if !canAccessWorkspace(req, workspaceID, userID) {
		return types2.NewErrHTTP(http.StatusForbidden, "Access denied to workspace")
	}

	// Verify workspace exists and user has permission to create ACRs
	workspace := &v1.PowerUserWorkspace{}
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      workspaceID,
	}, workspace); err != nil {
		if apierrors.IsNotFound(err) {
			return types2.NewErrHTTP(http.StatusNotFound, "Workspace not found")
		}
		return err
	}

	// Only PowerUserPlus and Admin can create ACRs
	if workspace.Spec.Role != types2.RolePowerUserPlus && workspace.Spec.Role != types2.RoleAdmin {
		return types2.NewErrHTTP(http.StatusForbidden, "Only PowerUser Plus and Admin roles can create Access Control Rules")
	}

	var acrReq types2.AccessControlRule
	if err := json.NewDecoder(req.Request.Body).Decode(&acrReq); err != nil {
		return types2.NewErrBadRequest("Invalid JSON: %v", err)
	}

	// Create AccessControlRule with workspace reference
	acr := &v1.AccessControlRule{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    system.DefaultNamespace,
			GenerateName: "workspace-acr-",
		},
		Spec: v1.AccessControlRuleSpec{
			MCPCatalogID:          system.DefaultCatalog, // Power users create ACRs in default catalog
			PowerUserWorkspaceID:  workspaceID,
			Manifest:              acrReq.AccessControlRuleManifest,
		},
	}

	if err := req.Storage.Create(req.Context(), acr); err != nil {
		return err
	}

	response := types2.AccessControlRule{
		Metadata: types2.Metadata{
			ID:      acr.Name,
			Created: *types2.NewTime(acr.CreationTimestamp.Time),
		},
		MCPCatalogID:              acr.Spec.MCPCatalogID,
		AccessControlRuleManifest: acr.Spec.Manifest,
	}

	return req.Write(response)
}

// Helper function to check if user can access workspace
func canAccessWorkspace(req api.Context, workspaceID, userID string) bool {
	// Admins can access any workspace
	if req.UserIsAdmin() {
		return true
	}

	// Users can only access their own workspace
	expectedWorkspaceName := fmt.Sprintf("workspace-%s", userID)
	return workspaceID == expectedWorkspaceName
}

// Helper function to check if user has role through groups
func userHasRole(req api.Context, role string) bool {
	for _, group := range req.User.GetGroups() {
		if group == role {
			return true
		}
	}
	return false
}
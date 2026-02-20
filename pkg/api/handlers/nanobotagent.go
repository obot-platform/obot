package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/wait"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type NanobotAgentHandler struct {
	sessionManager *mcp.SessionManager
	serverURL      string
}

func NewNanobotAgentHandler(sessionManager *mcp.SessionManager, serverURL string) *NanobotAgentHandler {
	return &NanobotAgentHandler{
		sessionManager: sessionManager,
		serverURL:      serverURL,
	}
}

func (h *NanobotAgentHandler) List(req api.Context) error {
	var agents v1.NanobotAgentList
	if err := req.List(&agents, kclient.MatchingFields{
		"spec.projectV2ID": req.PathValue("project_id"),
	}); err != nil {
		return err
	}

	items := make([]types.NanobotAgent, 0, len(agents.Items))
	for _, agent := range agents.Items {
		items = append(items, h.convertNanobotAgent(agent))
	}
	return req.Write(types.NanobotAgentList{Items: items})
}

func (h *NanobotAgentHandler) Create(req api.Context) error {
	var manifest types.NanobotAgentManifest
	if err := req.Read(&manifest); err != nil {
		return err
	}

	agent := v1.NanobotAgent{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.NanobotAgentPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.NanobotAgentSpec{
			NanobotAgentManifest: manifest,
			UserID:               req.User.GetUID(),
			ProjectV2ID:          req.PathValue("project_id"),
		},
	}

	if err := req.Create(&agent); err != nil {
		return err
	}

	return req.WriteCreated(h.convertNanobotAgent(agent))
}

func (h *NanobotAgentHandler) ByID(req api.Context) error {
	var agent v1.NanobotAgent
	if err := req.Get(&agent, req.PathValue("nanobot_agent_id")); err != nil {
		return err
	}

	// Ensure that the agent belongs to the specified project
	if agent.Spec.ProjectV2ID != req.PathValue("project_id") {
		return types.NewErrNotFound("nanobot agent not found")
	}

	return req.Write(h.convertNanobotAgent(agent))
}

func (h *NanobotAgentHandler) Update(req api.Context) error {
	var (
		id    = req.PathValue("nanobot_agent_id")
		agent v1.NanobotAgent
	)

	if err := req.Get(&agent, id); err != nil {
		return err
	}

	// Ensure that the agent belongs to the specified project
	if agent.Spec.ProjectV2ID != req.PathValue("project_id") {
		return types.NewErrNotFound("nanobot agent not found")
	}

	var manifest types.NanobotAgentManifest
	if err := req.Read(&manifest); err != nil {
		return err
	}

	agent.Spec.NanobotAgentManifest = manifest
	if err := req.Update(&agent); err != nil {
		return err
	}

	return req.Write(h.convertNanobotAgent(agent))
}

func (h *NanobotAgentHandler) Delete(req api.Context) error {
	var id = req.PathValue("nanobot_agent_id")
	var agent v1.NanobotAgent
	if err := req.Get(&agent, id); err != nil {
		return err
	}

	// Ensure that the agent belongs to the specified project
	if agent.Spec.ProjectV2ID != req.PathValue("project_id") {
		return types.NewErrNotFound("nanobot agent not found")
	}

	return req.Delete(&v1.NanobotAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      id,
			Namespace: req.Namespace(),
		},
	})
}

func (h *NanobotAgentHandler) Launch(req api.Context) error {
	var agent v1.NanobotAgent
	if err := req.Get(&agent, req.PathValue("nanobot_agent_id")); err != nil {
		return err
	}

	if agent.Spec.ProjectV2ID != req.PathValue("project_id") {
		return types.NewErrNotFound("nanobot agent not found")
	}

	server := &v1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: req.Namespace(),
			Name:      system.MCPServerPrefix + req.PathValue("nanobot_agent_id"),
		},
	}

	ctx, cancel := context.WithTimeout(req.Context(), 15*time.Second)
	defer cancel()

	server, err := wait.For(ctx, req.Storage, server, func(srv *v1.MCPServer) (bool, error) {
		return srv.ResourceVersion != "", nil
	}, wait.Option{
		WaitForExists: true,
	})
	if err != nil {
		return fmt.Errorf("failed to load MCP server for agent %s: %w", agent.Name, err)
	}

	serverConfig, err := serverConfigForAction(req, *server)
	if err != nil {
		return err
	}

	if _, err = h.sessionManager.LaunchServer(req.Context(), serverConfig); err != nil {
		if errors.Is(err, mcp.ErrHealthCheckFailed) || errors.Is(err, mcp.ErrHealthCheckTimeout) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, fmt.Sprintf("MCP server for agent %s is not healthy, check configuration for errors", agent.Name))
		}
		if errors.Is(err, nmcp.ErrNoResult) || strings.HasSuffix(err.Error(), nmcp.ErrNoResult.Error()) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, fmt.Sprintf("No response from MCP server for agent %s, check configuration for errors", agent.Name))
		}
		if errors.Is(err, mcp.ErrInsufficientCapacity) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "Insufficient capacity to deploy MCP server for agent. Please contact your administrator.")
		}
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
			return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
		}

		return fmt.Errorf("failed to launch MCP server for agent %s: %w", agent.Name, err)
	}

	return nil
}

func (h *NanobotAgentHandler) convertNanobotAgent(agent v1.NanobotAgent) types.NanobotAgent {
	return types.NanobotAgent{
		Metadata:             MetadataFrom(&agent),
		NanobotAgentManifest: agent.Spec.NanobotAgentManifest,
		UserID:               agent.Spec.UserID,
		ProjectV2ID:          agent.Spec.ProjectV2ID,
		ConnectURL:           system.NanobotAgentConnectURL(h.serverURL, agent.Name),
	}
}

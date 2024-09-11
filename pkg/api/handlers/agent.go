package handlers

import (
	"fmt"
	"net/http"

	"github.com/gptscript-ai/otto/pkg/api"
	"github.com/gptscript-ai/otto/pkg/api/types"
	v2 "github.com/gptscript-ai/otto/pkg/storage/apis/otto.gptscript.ai/v1"
	"github.com/thedadams/workspace-provider/pkg/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AgentHandler struct {
	workspaceClient   *client.Client
	workspaceProvider string
}

func NewAgentHandler(wc *client.Client, wp string) *AgentHandler {
	return &AgentHandler{
		workspaceClient:   wc,
		workspaceProvider: wp,
	}
}

func (a *AgentHandler) Update(req api.Context) error {
	var (
		id       = req.Request.PathValue("id")
		agent    v2.Agent
		manifest v2.AgentManifest
	)

	if err := req.Read(&manifest); err != nil {
		return err
	}

	if err := req.Get(&agent, id); err != nil {
		return err
	}

	agent.Spec.Manifest = manifest
	if err := req.Update(&agent); err != nil {
		return err
	}

	return req.Write(convertAgent(agent, api.GetURLPrefix(req)))
}

func (a *AgentHandler) Delete(req api.Context) error {
	var (
		id = req.Request.PathValue("id")
	)

	return req.Delete(&v2.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      id,
			Namespace: req.Namespace(),
		},
	})
}

func (a *AgentHandler) Create(req api.Context) error {
	var manifest v2.AgentManifest
	if err := req.Read(&manifest); err != nil {
		return err
	}
	agent := v2.Agent{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "a1",
			Namespace:    req.Namespace(),
		},
		Spec: v2.AgentSpec{
			Manifest: manifest,
		},
	}

	if err := req.Create(&agent); err != nil {
		return err
	}

	req.WriteHeader(http.StatusCreated)
	return req.Write(convertAgent(agent, api.GetURLPrefix(req)))
}

func convertAgent(agent v2.Agent, prefix string) *types.Agent {
	var links []string
	if prefix != "" {
		links = []string{"invoke", prefix + "/invoke/" + agent.Name}
	}
	return &types.Agent{
		Metadata:      types.MetadataFrom(&agent, links...),
		AgentManifest: agent.Spec.Manifest,
	}
}

func (a *AgentHandler) List(req api.Context) error {
	var agentList v2.AgentList
	if err := req.List(&agentList); err != nil {
		return err
	}

	var resp types.AgentList
	for _, agent := range agentList.Items {
		resp.Items = append(resp.Items, *convertAgent(agent, api.GetURLPrefix(req)))
	}

	return req.Write(resp)
}

func (a *AgentHandler) Files(req api.Context) error {
	var (
		id    = req.Request.PathValue("id")
		agent v2.Agent
	)
	if err := req.Get(&agent, id); err != nil {
		return fmt.Errorf("failed to get agent with id %s: %w", id, err)
	}

	return listFiles(req.Context(), req, a.workspaceClient, agent.Status.WorkspaceID)
}

func (a *AgentHandler) UploadFile(req api.Context) error {
	var (
		id    = req.Request.PathValue("id")
		agent v2.Agent
	)
	if err := req.Get(&agent, id); err != nil {
		return fmt.Errorf("failed to get agent with id %s: %w", id, err)
	}

	return uploadFile(req.Context(), req, a.workspaceClient, agent.Status.WorkspaceID)
}

func (a *AgentHandler) DeleteFile(req api.Context) error {
	var (
		id       = req.Request.PathValue("id")
		filename = req.Request.PathValue("file")
		agent    v2.Agent
	)

	if err := req.Get(&agent, id); err != nil {
		return fmt.Errorf("failed to get agent with id %s: %w", id, err)
	}

	return deleteFile(req.Context(), req, a.workspaceClient, agent.Status.WorkspaceID, filename)
}

func (a *AgentHandler) Knowledge(req api.Context) error {
	var (
		id    = req.Request.PathValue("id")
		agent v2.Agent
	)
	if err := req.Get(&agent, id); err != nil {
		return fmt.Errorf("failed to get agent with id %s: %w", id, err)
	}

	return listFiles(req.Context(), req, a.workspaceClient, agent.Status.KnowledgeWorkspaceID)
}

func (a *AgentHandler) UploadKnowledge(req api.Context) error {
	var (
		id    = req.Request.PathValue("id")
		agent v2.Agent
	)
	if err := req.Get(&agent, id); err != nil {
		return fmt.Errorf("failed to get agent with id %s: %w", id, err)
	}

	if err := uploadFile(req.Context(), req, a.workspaceClient, agent.Status.KnowledgeWorkspaceID); err != nil {
		return err
	}

	agent.Status.IngestKnowledge = true
	agent.Status.HasKnowledge = true
	return req.Storage.Status().Update(req.Context(), &agent)
}

func (a *AgentHandler) DeleteKnowledge(req api.Context) error {
	var (
		id       = req.Request.PathValue("id")
		filename = req.Request.PathValue("file")
		agent    v2.Agent
	)

	if err := req.Get(&agent, id); err != nil {
		return fmt.Errorf("failed to get agent with id %s: %w", id, err)
	}

	if err := deleteFile(req.Context(), req, a.workspaceClient, agent.Status.KnowledgeWorkspaceID, filename); err != nil {
		return err
	}

	agent.Status.IngestKnowledge = true
	return req.Storage.Status().Update(req.Context(), &agent)
}

func (a *AgentHandler) IngestKnowledge(req api.Context) error {
	var (
		id    = req.Request.PathValue("id")
		agent v2.Agent
	)
	if err := req.Get(&agent, id); err != nil {
		return fmt.Errorf("failed to get agent with id %s: %w", id, err)
	}

	files, err := a.workspaceClient.Ls(req.Context(), agent.Status.KnowledgeWorkspaceID)
	if err != nil {
		return err
	}

	req.WriteHeader(http.StatusNoContent)

	if len(files) == 0 && !agent.Status.HasKnowledge {
		return nil
	}

	agent.Status.IngestKnowledge = true
	return req.Storage.Status().Update(req.Context(), &agent)
}

package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/otto8-ai/otto8/apiclient/types"
	"github.com/otto8-ai/otto8/pkg/api"
	"github.com/otto8-ai/otto8/pkg/render"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.gptscript.ai/v1"
	"github.com/otto8-ai/otto8/pkg/system"
	"github.com/otto8-ai/workspace-provider/pkg/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
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
		id       = req.PathValue("id")
		agent    v1.Agent
		manifest types.AgentManifest
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
		id = req.PathValue("id")
	)

	return req.Delete(&v1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      id,
			Namespace: req.Namespace(),
		},
	})
}

func (a *AgentHandler) Create(req api.Context) error {
	var manifest types.AgentManifest
	if err := req.Read(&manifest); err != nil {
		return err
	}
	agent := v1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.AgentPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.AgentSpec{
			Manifest: manifest,
		},
	}

	if err := req.Create(&agent); err != nil {
		return err
	}

	req.WriteHeader(http.StatusCreated)
	return req.Write(convertAgent(agent, api.GetURLPrefix(req)))
}

func convertAgent(agent v1.Agent, prefix string) *types.Agent {
	var links []string
	if prefix != "" {
		refName := agent.Name
		if agent.Status.External.RefNameAssigned && agent.Spec.Manifest.RefName != "" {
			refName = agent.Spec.Manifest.RefName
		}
		links = []string{"invoke", prefix + "/invoke/" + refName}
	}
	return &types.Agent{
		Metadata:            MetadataFrom(&agent, links...),
		AgentManifest:       agent.Spec.Manifest,
		AgentExternalStatus: agent.Status.External,
	}
}

func (a *AgentHandler) ByID(req api.Context) error {
	var agent v1.Agent
	if err := req.Get(&agent, req.PathValue("id")); err != nil {
		return err
	}

	return req.Write(convertAgent(agent, api.GetURLPrefix(req)))
}

func (a *AgentHandler) List(req api.Context) error {
	var agentList v1.AgentList
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
		id    = req.PathValue("id")
		agent v1.Agent
	)
	if err := req.Get(&agent, id); err != nil {
		return fmt.Errorf("failed to get agent with id %s: %w", id, err)
	}

	return listFiles(req.Context(), req, a.workspaceClient, agent.Status.WorkspaceName)
}

func (a *AgentHandler) UploadFile(req api.Context) error {
	var (
		id    = req.PathValue("id")
		agent v1.Agent
	)
	if err := req.Get(&agent, id); err != nil {
		return fmt.Errorf("failed to get agent with id %s: %w", id, err)
	}

	if err := uploadFile(req.Context(), req, a.workspaceClient, agent.Status.WorkspaceName); err != nil {
		return err
	}

	req.WriteHeader(http.StatusCreated)
	return nil
}

func (a *AgentHandler) DeleteFile(req api.Context) error {
	var (
		id    = req.PathValue("id")
		agent v1.Agent
	)

	if err := req.Get(&agent, id); err != nil {
		return fmt.Errorf("failed to get agent with id %s: %w", id, err)
	}

	return deleteFile(req.Context(), req, a.workspaceClient, agent.Status.WorkspaceName)
}

func (a *AgentHandler) Knowledge(req api.Context) error {
	var agent v1.Agent
	if err := req.Get(&agent, req.PathValue("id")); err != nil {
		return err
	}
	return listKnowledgeFiles(req, agent.Status.KnowledgeSetNames...)
}

func (a *AgentHandler) UploadKnowledge(req api.Context) error {
	var agent v1.Agent
	if err := req.Get(&agent, req.PathValue("id")); err != nil {
		return err
	}
	return uploadKnowledge(req, a.workspaceClient, agent.Status.KnowledgeSetNames...)
}

func (a *AgentHandler) DeleteKnowledge(req api.Context) error {
	var agent v1.Agent
	if err := req.Get(&agent, req.PathValue("id")); err != nil {
		return err
	}
	return deleteKnowledge(req, req.PathValue("file"), agent.Status.KnowledgeSetNames...)
}

func (a *AgentHandler) CreateRemoteKnowledgeSource(req api.Context) error {
	return createRemoteKnowledgeSource(req, req.PathValue("agent_id"))
}

func (a *AgentHandler) UpdateRemoteKnowledgeSource(req api.Context) error {
	return updateRemoteKnowledgeSource(req, req.PathValue("id"), req.PathValue("agent_id"))
}

func (a *AgentHandler) ReSyncRemoteKnowledgeSource(req api.Context) error {
	return reSyncRemoteKnowledgeSource(req, req.PathValue("id"), req.PathValue("agent_id"))
}

func (a *AgentHandler) GetRemoteKnowledgeSources(req api.Context) error {
	return getRemoteKnowledgeSourceForParent(req, req.PathValue("agent_id"))
}

func (a *AgentHandler) DeleteRemoteKnowledgeSource(req api.Context) error {
	return deleteRemoteKnowledgeSource(req, req.PathValue("id"), req.PathValue("agent_id"))
}

func (a *AgentHandler) Script(req api.Context) error {
	var (
		id    = req.PathValue("id")
		agent v1.Agent
	)
	if err := req.Get(&agent, id); err != nil {
		return fmt.Errorf("failed to get agent with id %s: %w", id, err)
	}

	tools, extraEnv, err := render.Agent(req.Context(), req.Storage, &agent, render.AgentOptions{})
	if err != nil {
		return err
	}

	nodes := gptscript.ToolDefsToNodes(tools)
	nodes = append(nodes, gptscript.Node{
		TextNode: &gptscript.TextNode{
			Text: "!otto-extra-env\n" + strings.Join(extraEnv, "\n"),
		},
	})

	script, err := req.GPTClient.Fmt(req.Context(), nodes)
	if err != nil {
		return err
	}

	return req.Write(script)
}

func MetadataFrom(obj kclient.Object, linkKV ...string) types.Metadata {
	m := types.Metadata{
		ID:      obj.GetName(),
		Created: *types.NewTime(obj.GetCreationTimestamp().Time),
		Links:   map[string]string{},
	}
	if delTime := obj.GetDeletionTimestamp(); delTime != nil {
		m.Deleted = types.NewTime(delTime.Time)
	}
	for i := 0; i < len(linkKV); i += 2 {
		m.Links[linkKV[i]] = linkKV[i+1]
	}
	return m
}

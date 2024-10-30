package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/otto8-ai/otto8/apiclient/types"
	"github.com/otto8-ai/otto8/pkg/api"
	"github.com/otto8-ai/otto8/pkg/api/server"
	"github.com/otto8-ai/otto8/pkg/render"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.gptscript.ai/v1"
	"github.com/otto8-ai/otto8/pkg/storage/selectors"
	"github.com/otto8-ai/otto8/pkg/system"
	"github.com/otto8-ai/otto8/pkg/wait"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type AgentHandler struct {
	gptscript *gptscript.GPTScript
	serverURL string
}

func NewAgentHandler(gClient *gptscript.GPTScript, serverURL string) *AgentHandler {
	return &AgentHandler{
		serverURL: serverURL,
		gptscript: gClient,
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

	return req.Write(convertAgent(agent, server.GetURLPrefix(req)))
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
	return req.Write(convertAgent(agent, server.GetURLPrefix(req)))
}

func convertAgent(agent v1.Agent, prefix string, knowledgeSets ...v1.KnowledgeSet) *types.Agent {
	var links []string
	if prefix != "" {
		refName := agent.Name
		if agent.Status.RefNameAssigned && agent.Spec.Manifest.RefName != "" {
			refName = agent.Spec.Manifest.RefName
		}
		links = []string{"invoke", prefix + "/invoke/" + refName}
	}

	return &types.Agent{
		Metadata:        MetadataFrom(&agent, links...),
		AgentManifest:   agent.Spec.Manifest,
		RefNameAssigned: agent.Status.RefNameAssigned,
	}
}

func (a *AgentHandler) ByID(req api.Context) error {
	var agent v1.Agent
	if err := req.Get(&agent, req.PathValue("id")); err != nil {
		return err
	}

	knowledgeSets, err := getKnowledgeSetsFromAgent(req, agent)
	if err != nil {
		return err
	}

	return req.Write(convertAgent(agent, server.GetURLPrefix(req), knowledgeSets...))
}

func (a *AgentHandler) List(req api.Context) error {
	var agentList v1.AgentList
	if err := req.List(&agentList); err != nil {
		return err
	}

	var resp types.AgentList
	for _, agent := range agentList.Items {
		knowledgeSets, err := getKnowledgeSetsFromAgent(req, agent)
		if err != nil {
			return err
		}
		resp.Items = append(resp.Items, *convertAgent(agent, server.GetURLPrefix(req), knowledgeSets...))
	}

	return req.Write(resp)
}

func getKnowledgeSetsFromAgent(req api.Context, agent v1.Agent) ([]v1.KnowledgeSet, error) {
	var knowledgeSets v1.KnowledgeSetList
	if err := req.Storage.List(req.Context(), &knowledgeSets, &kclient.ListOptions{
		FieldSelector: fields.SelectorFromSet(selectors.RemoveEmpty(map[string]string{
			"spec.agentName": agent.Name,
		})),
		Namespace: agent.Namespace,
	}); err != nil {
		return nil, err
	}
	return knowledgeSets.Items, nil
}

func (a *AgentHandler) ListFiles(req api.Context) error {
	var (
		id    = req.PathValue("id")
		agent v1.Agent
	)
	if err := req.Get(&agent, id); err != nil {
		return types.NewErrBadRequest("failed to get agent with id %s: %w", id, err)
	}

	return listFiles(req.Context(), req, a.gptscript, agent.Status.WorkspaceName)
}

func (a *AgentHandler) UploadFile(req api.Context) error {
	var (
		id    = req.PathValue("id")
		agent v1.Agent
	)
	if err := req.Get(&agent, id); err != nil {
		return types.NewErrBadRequest("failed to get agent with id %s: %w", id, err)
	}

	if err := uploadFile(req.Context(), req, a.gptscript, agent.Status.WorkspaceName); err != nil {
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
		return types.NewErrBadRequest("failed to get agent with id %s: %w", id, err)
	}

	return deleteFile(req.Context(), req, a.gptscript, agent.Status.WorkspaceName, "files/")
}

func (a *AgentHandler) ListKnowledgeFiles(req api.Context) error {
	var agent v1.Agent
	if err := req.Get(&agent, req.PathValue("agent_id")); err != nil {
		return err
	}
	if len(agent.Status.KnowledgeSetNames) == 0 {
		return req.Write(types.KnowledgeFileList{Items: []types.KnowledgeFile{}})
	}

	knowledgeSourceName := req.PathValue("knowledge_source_id")
	if knowledgeSourceName != "" {
		var knowledgeSource v1.KnowledgeSource
		if err := req.Get(&knowledgeSource, knowledgeSourceName); err != nil {
			return err
		}
		if knowledgeSource.Spec.KnowledgeSetName != agent.Status.KnowledgeSetNames[0] {
			return types.NewErrBadRequest("knowledgeSource %q does not belong to agent %q", knowledgeSource.Name, agent.Name)
		}
	}

	return listKnowledgeFiles(req, agent.Name, "", agent.Status.KnowledgeSetNames[0], knowledgeSourceName)
}

func (a *AgentHandler) UploadKnowledgeFile(req api.Context) error {
	var agent v1.Agent
	if err := req.Get(&agent, req.PathValue("id")); err != nil {
		return err
	}
	if len(agent.Status.KnowledgeSetNames) == 0 {
		return types.NewErrHttp(http.StatusTooEarly, fmt.Sprintf("agent %q knowledge set is not created yet", agent.Name))
	}

	ws, err := getWorkspaceFromKnowledgeSet(req, agent.Status.KnowledgeSetNames[0])
	if err != nil {
		return err
	}

	return uploadKnowledgeToWorkspace(req, a.gptscript, ws, agent.Name, "", agent.Status.KnowledgeSetNames[0])
}

func (a *AgentHandler) ApproveKnowledgeFile(req api.Context) error {
	var body struct {
		Approved bool `json:"approved"`
	}

	if err := req.Read(&body); err != nil {
		return err
	}

	var file v1.KnowledgeFile
	if err := req.Get(&file, req.PathValue("file_id")); err != nil {
		return err
	}

	file.Spec.Approved = &body.Approved
	return req.Update(&file)
}

func (a *AgentHandler) DeleteKnowledgeFile(req api.Context) error {
	var agent v1.Agent
	if err := req.Get(&agent, req.PathValue("id")); err != nil {
		return err
	}
	if len(agent.Status.KnowledgeSetNames) == 0 {
		return types.NewErrHttp(http.StatusTooEarly, fmt.Sprintf("agent %q knowledge set is not created yet", agent.Name))
	}
	return deleteKnowledge(req, req.PathValue("file"), agent.Status.KnowledgeSetNames[0])
}

func (a *AgentHandler) CreateKnowledgeSource(req api.Context) error {
	var agent v1.Agent
	if err := req.Get(&agent, req.PathValue("agent_id")); err != nil {
		return err
	}

	if len(agent.Status.KnowledgeSetNames) == 0 {
		return types.NewErrBadRequest("agent %q knowledge set is not created yet", agent.Name)
	}

	var input types.KnowledgeSourceManifest
	if err := req.Read(&input); err != nil {
		return types.NewErrBadRequest("failed to decode request body: %w", err)
	}

	if err := input.Validate(); err != nil {
		return err
	}

	source := v1.KnowledgeSource{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    req.Namespace(),
			GenerateName: system.KnowledgeSourcePrefix,
			Finalizers:   []string{v1.KnowledgeSourceFinalizer},
		},
		Spec: v1.KnowledgeSourceSpec{
			KnowledgeSetName: agent.Status.KnowledgeSetNames[0],
			Manifest:         input,
		},
	}

	if err := req.Create(&source); err != nil {
		return types.NewErrBadRequest("failed to create RemoteKnowledgeSource: %w", err)
	}

	_, required, err := source.CredentialTool(req.Context(), req.Storage)
	if err != nil {
		return err
	}

	// Set the required field in the status so that the caller knows if the knowledge source needs to be authenticated.
	// This value will be persisted in the status in a controller.
	source.Status.Auth.Required = &required

	return req.Write(convertKnowledgeSource(agent.Name, source))
}

func (a *AgentHandler) UpdateKnowledgeSource(req api.Context) error {
	var agent v1.Agent
	if err := req.Get(&agent, req.PathValue("agent_id")); err != nil {
		return err
	}

	var manifest types.KnowledgeSourceManifest
	if err := req.Read(&manifest); err != nil {
		return types.NewErrBadRequest("failed to decode request body: %w", err)
	}

	if err := manifest.Validate(); err != nil {
		return err
	}

	if len(agent.Status.KnowledgeSetNames) == 0 {
		return types.NewErrHttp(http.StatusTooEarly, fmt.Sprintf("agent %q knowledge set is not created yet", agent.Name))
	}

	var knowledgeSource v1.KnowledgeSource
	if err := req.Get(&knowledgeSource, req.PathValue("id")); err != nil {
		return err
	}

	if knowledgeSource.Spec.KnowledgeSetName != agent.Status.KnowledgeSetNames[0] {
		return types.NewErrBadRequest("knowledgeSource %q does not belong to agent %q", knowledgeSource.Name, agent.Name)
	}

	if checkConfigChanged(knowledgeSource.Spec.Manifest.KnowledgeSourceInput, manifest.KnowledgeSourceInput) {
		knowledgeSource.Spec.SyncGeneration++
	}

	knowledgeSource.Spec.Manifest = manifest
	if err := req.Update(&knowledgeSource); err != nil {
		return err
	}

	return req.Write(convertKnowledgeSource(agent.Name, knowledgeSource))
}

func (a *AgentHandler) ReIngestKnowledgeFile(req api.Context) error {
	var agent v1.Agent
	if err := req.Get(&agent, req.PathValue("agent_id")); err != nil {
		return err
	}

	if len(agent.Status.KnowledgeSetNames) == 0 {
		return types.NewErrHttp(http.StatusTooEarly, fmt.Sprintf("agent %q knowledge set is not created yet", agent.Name))
	}

	var knowledgeSource v1.KnowledgeSource
	if err := req.Get(&knowledgeSource, req.PathValue("knowledge_source_id")); err != nil {
		return err
	}

	if knowledgeSource.Spec.KnowledgeSetName != agent.Status.KnowledgeSetNames[0] {
		return types.NewErrBadRequest("knowledgeSource %q does not belong to agent %q", knowledgeSource.Name, agent.Name)
	}

	var knowledgeFile v1.KnowledgeFile
	if err := req.Get(&knowledgeFile, req.PathValue("id")); err != nil {
		return err
	}

	if knowledgeFile.Spec.KnowledgeSourceName != knowledgeSource.Name {
		return types.NewErrBadRequest("knowledgeFile %q does not belong to knowledgeSource %q", knowledgeFile.Name, knowledgeSource.Name)
	}

	knowledgeFile.Spec.IngestGeneration++
	if err := req.Update(&knowledgeFile); err != nil {
		return err
	}

	knowledgeFile.Status.State = types.KnowledgeFileStatePending
	if err := req.Storage.Status().Update(req.Context(), &knowledgeFile); err != nil {
		return err
	}

	return req.Write(convertKnowledgeFile(agent.Name, "", knowledgeFile))
}

func (a *AgentHandler) ReSyncKnowledgeSource(req api.Context) error {
	var agent v1.Agent
	if err := req.Get(&agent, req.PathValue("agent_id")); err != nil {
		return err
	}

	if len(agent.Status.KnowledgeSetNames) == 0 {
		return types.NewErrHttp(http.StatusTooEarly, fmt.Sprintf("agent %q knowledge set is not created yet", agent.Name))
	}

	var knowledgeSource v1.KnowledgeSource
	if err := req.Get(&knowledgeSource, req.PathValue("id")); err != nil {
		return err
	}

	if knowledgeSource.Spec.KnowledgeSetName != agent.Status.KnowledgeSetNames[0] {
		return types.NewErrBadRequest("knowledgeSource %q does not belong to agent %q", knowledgeSource.Name, agent.Name)
	}

	knowledgeSource.Spec.SyncGeneration++
	if err := req.Update(&knowledgeSource); err != nil {
		return err
	}

	knowledgeSource.Status.SyncState = types.KnowledgeSourceStatePending
	knowledgeSource.Status.Status = ""
	if err := req.Storage.Status().Update(req.Context(), &knowledgeSource); err != nil {
		return err
	}

	req.WriteHeader(http.StatusNoContent)
	return nil
}

func (a *AgentHandler) ListKnowledgeSources(req api.Context) error {
	var agent v1.Agent
	if err := req.Get(&agent, req.PathValue("agent_id")); err != nil {
		return err
	}

	if len(agent.Status.KnowledgeSetNames) == 0 {
		return req.Write(types.KnowledgeSourceList{Items: []types.KnowledgeSource{}})
	}

	var knowledgeSourceList v1.KnowledgeSourceList
	if err := req.Storage.List(req.Context(), &knowledgeSourceList,
		kclient.InNamespace(req.Namespace()), kclient.MatchingFields{
			"spec.knowledgeSetName": agent.Status.KnowledgeSetNames[0],
		}); err != nil {
		return err
	}

	var resp []types.KnowledgeSource
	for _, source := range knowledgeSourceList.Items {
		resp = append(resp, convertKnowledgeSource(agent.Name, source))
	}

	return req.Write(types.KnowledgeSourceList{Items: resp})
}

func (a *AgentHandler) DeleteKnowledgeSource(req api.Context) error {
	var agent v1.Agent
	if err := req.Get(&agent, req.PathValue("agent_id")); err != nil {
		return err
	}

	if len(agent.Status.KnowledgeSetNames) == 0 {
		return types.NewErrHttp(http.StatusTooEarly, fmt.Sprintf("agent %q knowledge set is not created yet", agent.Name))
	}

	var knowledgeSource v1.KnowledgeSource
	if err := req.Get(&knowledgeSource, req.PathValue("id")); err != nil {
		return err
	}

	if knowledgeSource.Spec.KnowledgeSetName != agent.Status.KnowledgeSetNames[0] {
		return types.NewErrBadRequest("knowledgeSource %q does not belong to agent %q", knowledgeSource.Name, agent.Name)
	}

	return req.Delete(&v1.KnowledgeSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      knowledgeSource.Name,
			Namespace: req.Namespace(),
		},
	})
}

func (a *AgentHandler) EnsureCredentialForKnowledgeSource(req api.Context) error {
	var agent v1.Agent
	if err := req.Get(&agent, req.PathValue("agent_id")); err != nil {
		return err
	}

	if len(agent.Status.KnowledgeSetNames) == 0 {
		return types.NewErrHttp(http.StatusTooEarly, fmt.Sprintf("agent %q knowledge set is not created yet", agent.Name))
	}

	var knowledgeSource v1.KnowledgeSource
	if err := req.Get(&knowledgeSource, req.PathValue("id")); err != nil {
		return err
	}

	if knowledgeSource.Spec.KnowledgeSetName != agent.Status.KnowledgeSetNames[0] {
		return types.NewErrBadRequest("knowledgeSource %q does not belong to agent %q", knowledgeSource.Name, agent.Name)
	}

	// If auth is not required, then don't continue.
	if knowledgeSource.Status.Auth.Required != nil && !*knowledgeSource.Status.Auth.Required {
		return req.Write(convertKnowledgeSource(agent.Name, knowledgeSource))
	}

	credentialTool, required, err := knowledgeSource.CredentialTool(req.Context(), req.Storage)
	if err != nil {
		return err
	}

	if !required {
		// The only way to get here is if the controller hasn't set the field yet.
		knowledgeSource.Status.Auth.Required = &required
		return req.Write(convertKnowledgeSource(agent.Name, knowledgeSource))
	}

	if credentialTool == "" {
		return types.NewErrHttp(http.StatusTooEarly, fmt.Sprintf("knowledge source %q does not have credential tool", knowledgeSource.Name))
	}

	oauthLogin := &v1.OAuthAppLogin{
		ObjectMeta: metav1.ObjectMeta{
			Name:      system.OAuthAppLoginPrefix + knowledgeSource.Name,
			Namespace: req.Namespace(),
		},
		Spec: v1.OAuthAppLoginSpec{
			CredentialContext: knowledgeSource.Name,
			CredentialTool:    credentialTool,
		},
	}

	if err = req.Delete(oauthLogin); err != nil {
		return err
	}

	oauthLogin, err = wait.For(req.Context(), req.Storage, oauthLogin, func(obj *v1.OAuthAppLogin) bool {
		return obj.Status.Authenticated || obj.Status.Error != "" || obj.Status.URL != ""
	}, wait.Option{
		Create: true,
	})
	if err != nil {
		return fmt.Errorf("failed to ensure credential for knowledge source %q: %w", knowledgeSource.Name, err)
	}

	// Don't need to actually update the knowledge source, there is a controller that will do that.
	knowledgeSource.Status.Auth = oauthLogin.Status.OAuthAppLoginAuthStatus
	return req.Write(convertKnowledgeSource(agent.Name, knowledgeSource))
}

func (a *AgentHandler) Script(req api.Context) error {
	var (
		id    = req.PathValue("id")
		agent v1.Agent
	)
	if err := req.Get(&agent, id); err != nil {
		return types.NewErrBadRequest("failed to get agent with id %s: %w", id, err)
	}

	tools, extraEnv, err := render.Agent(req.Context(), req.Storage, &agent, a.serverURL, render.AgentOptions{})
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

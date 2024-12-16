package handlers

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/acorn-io/acorn/apiclient/types"
	"github.com/acorn-io/acorn/pkg/alias"
	"github.com/acorn-io/acorn/pkg/api"
	"github.com/acorn-io/acorn/pkg/render"
	v1 "github.com/acorn-io/acorn/pkg/storage/apis/otto.otto8.ai/v1"
	"github.com/acorn-io/acorn/pkg/system"
	"github.com/acorn-io/acorn/pkg/wait"
	"github.com/gptscript-ai/go-gptscript"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type AgentHandler struct {
	gptscript *gptscript.GPTScript
	serverURL string
	// This is currently a hack to access the workflow handler
	workflowHandler *WorkflowHandler
}

func NewAgentHandler(gClient *gptscript.GPTScript, serverURL string) *AgentHandler {
	return &AgentHandler{
		serverURL:       serverURL,
		gptscript:       gClient,
		workflowHandler: NewWorkflowHandler(gClient, serverURL, nil),
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

	if agent.Spec.Manifest.Model != manifest.Model && manifest.Model != "" {
		// Get the model to ensure it is active
		var model v1.Model
		if err := req.Get(&model, manifest.Model); err != nil {
			return err
		}

		if !model.Spec.Manifest.Active {
			return types.NewErrBadRequest("agent cannot use inactive model %q", manifest.Model)
		}
	}

	agent.Spec.Manifest = manifest
	if err := req.Update(&agent); err != nil {
		return err
	}

	var knowledgeSet v1.KnowledgeSet
	if len(agent.Status.KnowledgeSetNames) > 0 {
		if err := req.Get(&knowledgeSet, agent.Status.KnowledgeSetNames[0]); err != nil {
			return fmt.Errorf("failed to get agent knowledge set: %w", err)
		}
	}

	resp, err := convertAgent(agent, knowledgeSet.Status.TextEmbeddingModel, req.APIBaseURL)
	if err != nil {
		return err
	}
	return req.WriteCreated(resp)
}

func (a *AgentHandler) Delete(req api.Context) error {
	return req.Delete(&v1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.PathValue("id"),
			Namespace: req.Namespace(),
		},
	})
}

func (a *AgentHandler) Create(req api.Context) error {
	var manifest types.AgentManifest
	if err := req.Read(&manifest); err != nil {
		return err
	}

	if manifest.Model != "" {
		// Get the model to ensure it is active
		var model v1.Model
		if err := req.Get(&model, manifest.Model); err != nil {
			return err
		}

		if !model.Spec.Manifest.Active {
			return types.NewErrBadRequest("agent cannot use inactive model %q", manifest.Model)
		}
	}

	agent := &v1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.AgentPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.AgentSpec{
			Manifest: manifest,
		},
	}

	if err := req.Create(agent); err != nil {
		return err
	}

	// The agent won't have a knowledge set associated to it on create, so set the text embedding model to an empty string.
	resp, err := convertAgent(*agent, "", req.APIBaseURL)
	if err != nil {
		return err
	}

	return req.WriteCreated(resp)
}

func convertAgent(agent v1.Agent, textEmbeddingModel, baseURL string) (*types.Agent, error) {
	var links []string
	if baseURL != "" {
		alias := agent.Name
		if agent.Status.AliasAssigned && agent.Spec.Manifest.Alias != "" {
			alias = agent.Spec.Manifest.Alias
		}
		links = []string{"invoke", baseURL + "/invoke/" + alias}
	}

	var aliasAssigned *bool
	if agent.Generation == agent.Status.AliasObservedGeneration {
		aliasAssigned = &agent.Status.AliasAssigned
	}

	return &types.Agent{
		Metadata:           MetadataFrom(&agent, links...),
		AgentManifest:      agent.Spec.Manifest,
		AliasAssigned:      aliasAssigned,
		AuthStatus:         agent.Status.AuthStatus,
		TextEmbeddingModel: textEmbeddingModel,
	}, nil
}

func (a *AgentHandler) ByID(req api.Context) error {
	var agent v1.Agent
	if err := alias.Get(req.Context(), req.Storage, &agent, req.Namespace(), req.PathValue("id")); err != nil {
		return err
	}

	var knowledgeSet v1.KnowledgeSet
	if len(agent.Status.KnowledgeSetNames) > 0 {
		if err := req.Get(&knowledgeSet, agent.Status.KnowledgeSetNames[0]); err != nil {
			return fmt.Errorf("failed to get agent knowledge set: %w", err)
		}
	}

	resp, err := convertAgent(agent, knowledgeSet.Status.TextEmbeddingModel, req.APIBaseURL)
	if err != nil {
		return err
	}
	return req.WriteCreated(resp)
}

func (a *AgentHandler) List(req api.Context) error {
	var agentList v1.AgentList
	if err := req.List(&agentList); err != nil {
		return err
	}

	var knowledgeSets v1.KnowledgeSetList
	if err := req.List(&knowledgeSets); err != nil {
		return fmt.Errorf("failed to get agent knowledge sets: %w", err)
	}

	textEmbeddingModels := make(map[string]string, len(knowledgeSets.Items))
	for _, knowledgeSet := range knowledgeSets.Items {
		textEmbeddingModels[knowledgeSet.Name] = knowledgeSet.Status.TextEmbeddingModel
	}

	var textEmbeddingModel string
	resp := make([]types.Agent, 0, len(agentList.Items))
	for _, agent := range agentList.Items {
		if len(agent.Status.KnowledgeSetNames) != 0 {
			textEmbeddingModel = textEmbeddingModels[agent.Status.KnowledgeSetNames[0]]
		} else {
			textEmbeddingModel = ""
		}
		convertedAgent, err := convertAgent(agent, textEmbeddingModel, req.APIBaseURL)
		if err != nil {
			return err
		}
		resp = append(resp, *convertedAgent)
	}

	return req.Write(types.AgentList{Items: resp})
}

func (a *AgentHandler) getWorkspaceName(req api.Context, agentOrWorkflowName string) (string, error) {
	if system.IsWorkflowID(agentOrWorkflowName) {
		var wf v1.Workflow
		if err := req.Get(&wf, agentOrWorkflowName); err != nil {
			return "", err
		}
		return wf.Status.WorkspaceName, nil
	}

	var agent v1.Agent
	if err := req.Get(&agent, agentOrWorkflowName); err != nil {
		return "", err
	}

	return agent.Status.WorkspaceName, nil
}

func (a *AgentHandler) ListFiles(req api.Context) error {
	workspaceName, err := a.getWorkspaceName(req, req.PathValue("id"))
	if err != nil {
		return err
	}

	return listFiles(req.Context(), req, a.gptscript, workspaceName)
}

func (a *AgentHandler) UploadFile(req api.Context) error {
	workspaceName, err := a.getWorkspaceName(req, req.PathValue("id"))
	if err != nil {
		return err
	}

	if err := uploadFile(req.Context(), req, a.gptscript, workspaceName); err != nil {
		return err
	}

	req.WriteHeader(http.StatusCreated)
	return nil
}

func (a *AgentHandler) DeleteFile(req api.Context) error {
	workspaceName, err := a.getWorkspaceName(req, req.PathValue("id"))
	if err != nil {
		return err
	}

	return deleteFile(req.Context(), req, a.gptscript, workspaceName, "files/")
}

func (a *AgentHandler) getKnowledgeSetsAndName(req api.Context, agentOrWorkflowName string) ([]string, string, error) {
	if system.IsWorkflowID(agentOrWorkflowName) {
		var wf v1.Workflow
		if err := req.Get(&wf, agentOrWorkflowName); err != nil {
			return nil, "", err
		}
		return wf.Status.KnowledgeSetNames, wf.Name, nil
	}

	var agent v1.Agent
	if err := req.Get(&agent, agentOrWorkflowName); err != nil {
		return nil, "", err
	}

	return agent.Status.KnowledgeSetNames, agent.Name, nil
}

func (a *AgentHandler) ListKnowledgeFiles(req api.Context) error {
	knowledgeSetNames, agentName, err := a.getKnowledgeSetsAndName(req, req.PathValue("agent_id"))
	if err != nil {
		return err
	}

	if len(knowledgeSetNames) == 0 {
		return req.Write(types.KnowledgeFileList{Items: []types.KnowledgeFile{}})
	}

	knowledgeSourceName := req.PathValue("knowledge_source_id")
	var knowledgeSource *v1.KnowledgeSource
	if knowledgeSourceName != "" {
		knowledgeSource = &v1.KnowledgeSource{}
		if err := req.Get(knowledgeSource, knowledgeSourceName); err != nil {
			return err
		}
		if knowledgeSource.Spec.KnowledgeSetName != knowledgeSetNames[0] {
			return types.NewErrBadRequest("knowledgeSource %q does not belong to agent %q", knowledgeSource.Name, agentName)
		}
	}

	return listKnowledgeFiles(req, agentName, "", knowledgeSetNames[0], knowledgeSource)
}

func (a *AgentHandler) UploadKnowledgeFile(req api.Context) error {
	knowledgeSetNames, agentName, err := a.getKnowledgeSetsAndName(req, req.PathValue("id"))
	if err != nil {
		return err
	}

	if len(knowledgeSetNames) == 0 {
		return types.NewErrHttp(http.StatusTooEarly, fmt.Sprintf("agent %q knowledge set is not created yet", agentName))
	}

	ws, err := getWorkspaceFromKnowledgeSet(req, knowledgeSetNames[0])
	if err != nil {
		return err
	}

	return uploadKnowledgeToWorkspace(req, a.gptscript, ws, agentName, "", knowledgeSetNames[0])
}

func (a *AgentHandler) ApproveKnowledgeFile(req api.Context) error {
	_, agentName, err := a.getKnowledgeSetsAndName(req, req.PathValue("agent_id"))
	if err != nil {
		return err
	}

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
	if err := req.Update(&file); err != nil {
		return err
	}

	return req.Write(convertKnowledgeFile(agentName, "", file))
}

func (a *AgentHandler) DeleteKnowledgeFile(req api.Context) error {
	knowledgeSetNames, agentName, err := a.getKnowledgeSetsAndName(req, req.PathValue("id"))
	if err != nil {
		return err
	}

	if len(knowledgeSetNames) == 0 {
		return types.NewErrHttp(http.StatusTooEarly, fmt.Sprintf("agent %q knowledge set is not created yet", agentName))
	}
	return deleteKnowledge(req, req.PathValue("file"), knowledgeSetNames[0])
}

func (a *AgentHandler) CreateKnowledgeSource(req api.Context) error {
	knowledgeSetNames, agentName, err := a.getKnowledgeSetsAndName(req, req.PathValue("agent_id"))
	if err != nil {
		return err
	}

	if len(knowledgeSetNames) == 0 {
		return types.NewErrBadRequest("agent %q knowledge set is not created yet", agentName)
	}

	var input types.KnowledgeSourceManifest
	if err := req.Read(&input); err != nil {
		return types.NewErrBadRequest("failed to decode request body: %v", err)
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
			KnowledgeSetName: knowledgeSetNames[0],
			Manifest:         input,
		},
	}

	if err := req.Create(&source); err != nil {
		return types.NewErrBadRequest("failed to create RemoteKnowledgeSource: %v", err)
	}

	return req.Write(convertKnowledgeSource(agentName, source))
}

func (a *AgentHandler) UpdateKnowledgeSource(req api.Context) error {
	knowledgeSetNames, agentName, err := a.getKnowledgeSetsAndName(req, req.PathValue("agent_id"))
	if err != nil {
		return err
	}

	var manifest types.KnowledgeSourceManifest
	if err := req.Read(&manifest); err != nil {
		return types.NewErrBadRequest("failed to decode request body: %v", err)
	}

	if err := manifest.Validate(); err != nil {
		return err
	}

	if len(knowledgeSetNames) == 0 {
		return types.NewErrHttp(http.StatusTooEarly, fmt.Sprintf("agent %q knowledge set is not created yet", agentName))
	}

	var knowledgeSource v1.KnowledgeSource
	if err := req.Get(&knowledgeSource, req.PathValue("id")); err != nil {
		return err
	}

	if knowledgeSource.Spec.KnowledgeSetName != knowledgeSetNames[0] {
		return types.NewErrBadRequest("knowledgeSource %q does not belong to agent %q", knowledgeSource.Name, agentName)
	}

	if checkConfigChanged(knowledgeSource.Spec.Manifest.KnowledgeSourceInput, manifest.KnowledgeSourceInput) {
		knowledgeSource.Spec.SyncGeneration++
	}

	knowledgeSource.Spec.Manifest = manifest
	if err := req.Update(&knowledgeSource); err != nil {
		return err
	}

	return req.Write(convertKnowledgeSource(agentName, knowledgeSource))
}

func (a *AgentHandler) ReIngestKnowledgeFile(req api.Context) error {
	knowledgeSetNames, agentName, err := a.getKnowledgeSetsAndName(req, req.PathValue("agent_id"))
	if err != nil {
		return err
	}

	if len(knowledgeSetNames) == 0 {
		return types.NewErrHttp(http.StatusTooEarly, fmt.Sprintf("agent %q knowledge set is not created yet", agentName))
	}

	var knowledgeFile v1.KnowledgeFile
	if err := req.Get(&knowledgeFile, req.PathValue("file_id")); err != nil {
		return err
	}

	if req.PathValue("knowledge_source_id") != "" {
		var knowledgeSource v1.KnowledgeSource
		if err := req.Get(&knowledgeSource, req.PathValue("knowledge_source_id")); err != nil {
			return err
		}

		if knowledgeSource.Spec.KnowledgeSetName != knowledgeSetNames[0] {
			return types.NewErrBadRequest("knowledgeSource %q does not belong to agent %q", knowledgeSource.Name, agentName)
		}

		if knowledgeFile.Spec.KnowledgeSourceName != knowledgeSource.Name {
			return types.NewErrBadRequest("knowledgeFile %q does not belong to knowledgeSource %q", knowledgeFile.Name, knowledgeSource.Name)
		}
	}

	knowledgeFile.Spec.IngestGeneration++
	if err := req.Update(&knowledgeFile); err != nil {
		return err
	}

	knowledgeFile.Status.State = types.KnowledgeFileStatePending
	knowledgeFile.Status.Error = ""
	if err := req.Storage.Status().Update(req.Context(), &knowledgeFile); err != nil {
		return err
	}

	return req.Write(convertKnowledgeFile(agentName, "", knowledgeFile))
}

func (a *AgentHandler) ReSyncKnowledgeSource(req api.Context) error {
	knowledgeSetNames, agentName, err := a.getKnowledgeSetsAndName(req, req.PathValue("agent_id"))
	if err != nil {
		return err
	}

	if len(knowledgeSetNames) == 0 {
		return types.NewErrHttp(http.StatusTooEarly, fmt.Sprintf("agent %q knowledge set is not created yet", agentName))
	}

	var knowledgeSource v1.KnowledgeSource
	if err := req.Get(&knowledgeSource, req.PathValue("id")); err != nil {
		return err
	}

	if knowledgeSource.Spec.KnowledgeSetName != knowledgeSetNames[0] {
		return types.NewErrBadRequest("knowledgeSource %q does not belong to agent %q", knowledgeSource.Name, agentName)
	}

	knowledgeSource.Spec.SyncGeneration++
	if err := req.Update(&knowledgeSource); err != nil {
		return err
	}

	knowledgeSource.Status.SyncState = types.KnowledgeSourceStatePending
	if err := req.Storage.Status().Update(req.Context(), &knowledgeSource); err != nil {
		return err
	}

	return req.Write(convertKnowledgeSource(agentName, knowledgeSource))
}

func (a *AgentHandler) ListKnowledgeSources(req api.Context) error {
	knowledgeSetNames, agentName, err := a.getKnowledgeSetsAndName(req, req.PathValue("agent_id"))
	if err != nil {
		return err
	}

	if len(knowledgeSetNames) == 0 {
		return req.Write(types.KnowledgeSourceList{Items: []types.KnowledgeSource{}})
	}

	var knowledgeSourceList v1.KnowledgeSourceList
	if err := req.Storage.List(req.Context(), &knowledgeSourceList,
		kclient.InNamespace(req.Namespace()), kclient.MatchingFields{
			"spec.knowledgeSetName": knowledgeSetNames[0],
		}); err != nil {
		return err
	}

	var resp []types.KnowledgeSource
	for _, source := range knowledgeSourceList.Items {
		resp = append(resp, convertKnowledgeSource(agentName, source))
	}

	return req.Write(types.KnowledgeSourceList{Items: resp})
}

func (a *AgentHandler) DeleteKnowledgeSource(req api.Context) error {
	knowledgeSetNames, agentName, err := a.getKnowledgeSetsAndName(req, req.PathValue("agent_id"))
	if err != nil {
		return err
	}

	if len(knowledgeSetNames) == 0 {
		return types.NewErrHttp(http.StatusTooEarly, fmt.Sprintf("agent %q knowledge set is not created yet", agentName))
	}

	var knowledgeSource v1.KnowledgeSource
	if err := req.Get(&knowledgeSource, req.PathValue("id")); err != nil {
		return err
	}

	if knowledgeSource.Spec.KnowledgeSetName != knowledgeSetNames[0] {
		return types.NewErrBadRequest("knowledgeSource %q does not belong to agent %q", knowledgeSource.Name, agentName)
	}

	return req.Delete(&v1.KnowledgeSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      knowledgeSource.Name,
			Namespace: req.Namespace(),
		},
	})
}

func (a *AgentHandler) EnsureCredentialForKnowledgeSource(req api.Context) error {
	agentID := req.PathValue("id")
	if system.IsWorkflowID(agentID) {
		return a.workflowHandler.EnsureCredentialForKnowledgeSource(req)
	}

	var agent v1.Agent
	if err := req.Get(&agent, agentID); err != nil {
		return err
	}

	var knowledgeSet v1.KnowledgeSet
	if len(agent.Status.KnowledgeSetNames) != 0 {
		if err := req.Get(&knowledgeSet, agent.Status.KnowledgeSetNames[0]); err != nil {
			return err
		}
	}

	ref := req.PathValue("ref")
	authStatus := agent.Status.AuthStatus[ref]

	// If auth is not required, then don't continue.
	if authStatus.Required != nil && !*authStatus.Required {
		resp, err := convertAgent(agent, knowledgeSet.Status.TextEmbeddingModel, req.APIBaseURL)
		if err != nil {
			return err
		}
		return req.WriteCreated(resp)
	}

	// if auth is already authenticated, then don't continue.
	if authStatus.Authenticated {
		resp, err := convertAgent(agent, knowledgeSet.Status.TextEmbeddingModel, req.APIBaseURL)
		if err != nil {
			return err
		}
		return req.WriteCreated(resp)
	}

	credentialTool, err := v1.CredentialTool(req.Context(), req.Storage, req.Namespace(), ref)
	if err != nil {
		return err
	}

	if credentialTool == "" {
		// The only way to get here is if the controller hasn't set the field yet.
		if agent.Status.AuthStatus == nil {
			agent.Status.AuthStatus = make(map[string]types.OAuthAppLoginAuthStatus)
		}

		authStatus.Required = &[]bool{false}[0]
		agent.Status.AuthStatus[ref] = authStatus
		resp, err := convertAgent(agent, knowledgeSet.Status.TextEmbeddingModel, req.APIBaseURL)
		if err != nil {
			return err
		}
		return req.WriteCreated(resp)
	}

	oauthLogin := &v1.OAuthAppLogin{
		ObjectMeta: metav1.ObjectMeta{
			Name:      system.OAuthAppLoginPrefix + agent.Name + ref,
			Namespace: req.Namespace(),
		},
		Spec: v1.OAuthAppLoginSpec{
			CredentialContext: agent.Name,
			ToolReference:     ref,
			OAuthApps:         agent.Spec.Manifest.OAuthApps,
		},
	}

	if err = req.Delete(oauthLogin); err != nil {
		return err
	}

	oauthLogin, err = wait.For(req.Context(), req.Storage, oauthLogin, func(obj *v1.OAuthAppLogin) (bool, error) {
		return obj.Status.External.Authenticated || obj.Status.External.Error != "" || obj.Status.External.URL != "", nil
	}, wait.Option{
		Create: true,
	})
	if err != nil {
		return fmt.Errorf("failed to ensure credential for agent %q: %w", agent.Name, err)
	}

	// Don't need to actually update the knowledge ref, there is a controller that will do that.
	if agent.Status.AuthStatus == nil {
		agent.Status.AuthStatus = make(map[string]types.OAuthAppLoginAuthStatus)
	}
	agent.Status.AuthStatus[ref] = oauthLogin.Status.External

	resp, err := convertAgent(agent, knowledgeSet.Status.TextEmbeddingModel, req.APIBaseURL)
	if err != nil {
		return err
	}
	return req.WriteCreated(resp)
}

func (a *AgentHandler) Script(req api.Context) error {
	var (
		id    = req.PathValue("id")
		agent v1.Agent
	)
	if err := req.Get(&agent, id); err != nil {
		return types.NewErrBadRequest("failed to get agent with id %s: %v", id, err)
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
		ID:       obj.GetName(),
		Created:  *types.NewTime(obj.GetCreationTimestamp().Time),
		Links:    map[string]string{},
		Type:     strings.ToLower(reflect.TypeOf(obj).Elem().Name()),
		Revision: obj.GetResourceVersion(),
	}
	if delTime := obj.GetDeletionTimestamp(); delTime != nil {
		m.Deleted = types.NewTime(delTime.Time)
	}
	for i := 0; i < len(linkKV); i += 2 {
		m.Links[linkKV[i]] = linkKV[i+1]
	}
	return m
}

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/alias"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/controller/creds"
	"github.com/obot-platform/obot/pkg/invoke"
	"github.com/obot-platform/obot/pkg/render"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/selectors"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/wait"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type AgentHandler struct {
	gptscript *gptscript.GPTScript
	invoker   *invoke.Invoker
	serverURL string
	// This is currently a hack to access the workflow handler
	workflowHandler *WorkflowHandler
}

func NewAgentHandler(gClient *gptscript.GPTScript, invoker *invoke.Invoker, serverURL string) *AgentHandler {
	return &AgentHandler{
		serverURL:       serverURL,
		gptscript:       gClient,
		invoker:         invoker,
		workflowHandler: NewWorkflowHandler(gClient, serverURL, invoker),
	}
}

func (a *AgentHandler) Authenticate(req api.Context) (err error) {
	var (
		id    = req.PathValue("id")
		agent v1.Agent
		tools []string
	)

	if err := req.Read(&tools); err != nil {
		return fmt.Errorf("failed to read tools from request body: %w", err)
	}

	if len(tools) == 0 {
		return types.NewErrBadRequest("no tools provided for authentication")
	}

	if err = req.Get(&agent, id); err != nil {
		return err
	}

	resp, err := runAuthForAgent(req.Context(), req.Storage, a.invoker, a.gptscript, agent.DeepCopy(), id, tools)
	if err != nil {
		return err
	}
	defer func() {
		resp.Close()
		if kickErr := kickAgent(req.Context(), req.Storage, &agent); kickErr != nil && err == nil {
			err = fmt.Errorf("failed to update agent status: %w", kickErr)
		}
	}()

	req.ResponseWriter.Header().Set("X-Obot-Thread-Id", resp.Thread.Name)
	return req.WriteEvents(resp.Events)
}

func (a *AgentHandler) DeAuthenticate(req api.Context) error {
	var (
		id    = req.PathValue("id")
		agent v1.Agent
		tools []string
	)

	if err := req.Read(&tools); err != nil {
		return fmt.Errorf("failed to read tools from request body: %w", err)
	}

	if len(tools) == 0 {
		return types.NewErrBadRequest("no tools provided for de-authentication")
	}

	if err := req.Get(&agent, id); err != nil {
		return err
	}

	errs := removeToolCredentials(req.Context(), req.Storage, a.gptscript, id, agent.Namespace, tools)

	if err := kickAgent(req.Context(), req.Storage, &agent); err != nil {
		errs = append(errs, fmt.Errorf("failed to update agent status: %w", err))
	}

	return errors.Join(errs...)
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

	var (
		aliasAssigned *bool
		toolInfos     *map[string]types.ToolInfo
	)
	if agent.Generation == agent.Status.ObservedGeneration {
		aliasAssigned = &agent.Status.AliasAssigned
		toolInfos = &agent.Status.ToolInfo
	}

	return &types.Agent{
		Metadata:           MetadataFrom(&agent, links...),
		AgentManifest:      agent.Spec.Manifest,
		AliasAssigned:      aliasAssigned,
		AuthStatus:         agent.Status.AuthStatus,
		ToolInfo:           toolInfos,
		TextEmbeddingModel: textEmbeddingModel,
	}, nil
}

func (a *AgentHandler) SetDefault(req api.Context) error {
	var newDefault v1.Agent
	if err := alias.Get(req.Context(), req.Storage, &newDefault, req.Namespace(), req.PathValue("id")); err != nil {
		return err
	}

	var agents v1.AgentList
	if err := req.List(&agents); err != nil {
		return err
	}

	var knowledgeSet v1.KnowledgeSet
	if len(newDefault.Status.KnowledgeSetNames) > 0 {
		if err := req.Get(&knowledgeSet, newDefault.Status.KnowledgeSetNames[0]); err != nil {
			return fmt.Errorf("failed to get agent knowledge set: %w", err)
		}
	}

	if !newDefault.Spec.Manifest.Default {
		newDefault.Spec.Manifest.Default = true
		if err := req.Update(&newDefault); err != nil {
			return err
		}
	}

	var errs []error
	for _, agent := range agents.Items {
		if newDefault.Name != agent.Name && agent.Spec.Manifest.Default {
			agent.Spec.Manifest.Default = false
			if err := req.Update(&agent); err != nil {
				errs = append(errs, err)
			}
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	resp, err := convertAgent(newDefault, knowledgeSet.Status.TextEmbeddingModel, req.APIBaseURL)
	if err != nil {
		return err
	}

	return req.WriteCreated(resp)
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

func (a *AgentHandler) GetFile(req api.Context) error {
	var (
		agentID = req.PathValue("id")
	)

	var agent v1.Agent
	if err := req.Get(&agent, agentID); err != nil {
		return err
	}

	var workspace v1.Workspace
	if err := req.Get(&workspace, agent.Status.WorkspaceName); err != nil {
		return err
	}

	return getFileInWorkspace(req.Context(), req, a.gptscript, workspace.Status.WorkspaceID, "files/")
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

	credentialTools, err := v1.CredentialTools(req.Context(), req.Storage, req.Namespace(), ref)
	if err != nil {
		return err
	}

	if len(credentialTools) == 0 {
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
		id       = req.PathValue("id")
		threadID = req.PathValue("thread_id")
		agent    v1.Agent
		thread   *v1.Thread
	)
	if err := req.Get(&agent, id); err != nil {
		return types.NewErrBadRequest("failed to get agent with id %s: %v", id, err)
	}
	if threadID != "" {
		thread = &v1.Thread{}
		if err := req.Get(thread, threadID); err != nil {
			return types.NewErrBadRequest("failed to get thread with id %s: %v", threadID, err)
		}
	}

	tools, extraEnv, err := render.Agent(req.Context(), req.Storage, &agent, a.serverURL, render.AgentOptions{
		Thread: thread,
	})
	if err != nil {
		return err
	}

	nodes := gptscript.ToolDefsToNodes(tools)
	nodes = append(nodes, gptscript.Node{
		TextNode: &gptscript.TextNode{
			Text: "!obot-extra-env\n" + strings.Join(extraEnv, "\n"),
		},
	})

	script, err := req.GPTClient.Fmt(req.Context(), nodes)
	if err != nil {
		return err
	}

	return req.Write(script)
}

func (a *AgentHandler) WatchKnowledgeFile(req api.Context) error {
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

	w, err := req.Storage.Watch(req.Context(), &v1.KnowledgeFileList{}, kclient.InNamespace(req.Namespace()),
		&kclient.ListOptions{
			FieldSelector: fields.SelectorFromSet(selectors.RemoveEmpty(map[string]string{
				"spec.knowledgeSetName":    knowledgeSetNames[0],
				"spec.knowledgeSourceName": knowledgeSourceName,
			})),
		})
	if err != nil {
		return err
	}
	defer func() {
		w.Stop()
		//nolint:revive
		for range w.ResultChan() {
		}
	}()

	req.ResponseWriter.Header().Set("Content-Type", "text/event-stream")
	defer func() {
		_ = req.WriteDataEvent(api.EventClose{})
	}()

	for event := range w.ResultChan() {
		if knowledgeFile, ok := event.Object.(*v1.KnowledgeFile); ok {
			payload := map[string]any{
				"eventType":     event.Type,
				"knowledgeFile": convertKnowledgeFile(agentName, "", *knowledgeFile),
			}
			data, err := json.Marshal(payload)
			if err != nil {
				return err
			}
			sseEvent := fmt.Sprintf("data: %s\n\n", data)
			if _, err := req.ResponseWriter.Write([]byte(sseEvent)); err != nil {
				return err
			}
			req.Flush()
		}
	}

	return nil
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

func runAuthForAgent(ctx context.Context, c kclient.WithWatch, invoker *invoke.Invoker, gClient *gptscript.GPTScript, agent *v1.Agent, credContext string, tools []string) (*invoke.Response, error) {
	credentials := make([]string, 0, len(tools))

	var toolRef v1.ToolReference
	for _, tool := range tools {
		if strings.ContainsAny(tool, "./") {
			prg, err := gClient.LoadFile(ctx, tool)
			if err != nil {
				return nil, err
			}

			credentails, _, err := creds.DetermineCredsAndCredNames(prg, prg.ToolSet[prg.EntryToolID], tool)
			if err != nil {
				return nil, err
			}

			credentials = append(credentials, credentails...)
		} else if err := c.Get(ctx, kclient.ObjectKey{Namespace: agent.Namespace, Name: tool}, &toolRef); err == nil {
			if toolRef.Status.Tool == nil {
				return nil, types.NewErrHttp(http.StatusTooEarly, fmt.Sprintf("tool %q is not ready", tool))
			}

			credentials = append(credentials, toolRef.Status.Tool.Credentials...)

			// Reset the fields we care about so that we can use the same variable for the whole loop.
			toolRef.Status.Tool = nil
		} else {
			return nil, err
		}
	}

	agent.Spec.Manifest.Prompt = "#!sys.echo\nDONE"
	agent.Spec.Manifest.Tools = tools
	agent.Spec.Manifest.AvailableThreadTools = nil
	agent.Spec.Manifest.DefaultThreadTools = nil
	agent.Spec.Credentials = credentials
	agent.Spec.CredentialContextID = credContext
	agent.Name = ""

	return invoker.Agent(ctx, c, agent, "", invoke.Options{
		Synchronous:           true,
		ThreadCredentialScope: new(bool),
	})
}

func removeToolCredentials(ctx context.Context, client kclient.Client, gClient *gptscript.GPTScript, credCtx, namespace string, tools []string) []error {
	var (
		errs            []error
		toolRef         v1.ToolReference
		credentialNames []string
	)
	for _, tool := range tools {
		if strings.ContainsAny(tool, "./") {
			prg, err := gClient.LoadFile(ctx, tool)
			if err != nil {
				errs = append(errs, err)
				continue
			}

			_, names, err := creds.DetermineCredsAndCredNames(prg, prg.ToolSet[prg.EntryToolID], tool)
			if err != nil {
				errs = append(errs, err)
				continue
			}

			credentialNames = append(credentialNames, names...)
		} else if err := client.Get(ctx, kclient.ObjectKey{Namespace: namespace, Name: tool}, &toolRef); err == nil {
			if toolRef.Status.Tool != nil {
				credentialNames = append(credentialNames, toolRef.Status.Tool.CredentialNames...)
			}
		} else {
			errs = append(errs, err)
			continue
		}

		// Reset the value we care about so the same variable can be used.
		// This ensures that the value we read on the next iteration is pulled from the database.
		toolRef.Status.Tool = nil

		for _, cred := range credentialNames {
			if err := gClient.DeleteCredential(ctx, credCtx, cred); err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
				errs = append(errs, err)
			}
		}
	}

	return errs
}

func kickAgent(ctx context.Context, c kclient.Client, agent *v1.Agent) error {
	if agent.Annotations[v1.AgentSyncAnnotation] != "" {
		delete(agent.Annotations, v1.AgentSyncAnnotation)
	} else {
		if agent.Annotations == nil {
			agent.Annotations = make(map[string]string)
		}
		agent.Annotations[v1.AgentSyncAnnotation] = "true"
	}

	return c.Update(ctx, agent)
}

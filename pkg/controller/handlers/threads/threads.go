package threads

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/nah/pkg/randomtoken"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/create"
	"github.com/obot-platform/obot/pkg/invoke"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	gptScript *gptscript.GPTScript
	invoker   *invoke.Invoker
}

func NewHandler(gptScript *gptscript.GPTScript, invoker *invoke.Invoker) *Handler {
	return &Handler{gptScript: gptScript, invoker: invoker}
}

func (t *Handler) WorkflowState(req router.Request, _ router.Response) error {
	var (
		thread = req.Object.(*v1.Thread)
		wfe    v1.WorkflowExecution
	)

	if thread.Spec.WorkflowExecutionName != "" {
		if err := req.Get(&wfe, thread.Namespace, thread.Spec.WorkflowExecutionName); err != nil {
			return err
		}
		thread.Status.WorkflowState = wfe.Status.State
	}

	return nil
}

func getParentWorkspaceNames(ctx context.Context, c kclient.Client, thread *v1.Thread) ([]string, bool, error) {
	var result []string

	if thread.Spec.Project {
		// Projects don't copy the parents/agent workspace unless it is a copy of another project
		if thread.Spec.SourceThreadName != "" {
			var sourceThread v1.Thread
			if err := c.Get(ctx, kclient.ObjectKey{Namespace: thread.Namespace, Name: thread.Spec.SourceThreadName}, &sourceThread); err != nil {
				return nil, false, err
			}
			if sourceThread.Status.WorkspaceName == "" {
				return nil, false, nil
			}
			return []string{sourceThread.Status.WorkspaceName}, true, nil
		}
		return nil, true, nil
	}

	parentThreadName := thread.Spec.ParentThreadName
	for parentThreadName != "" {
		var parentThread v1.Thread
		if err := c.Get(ctx, kclient.ObjectKey{Namespace: thread.Namespace, Name: parentThreadName}, &parentThread); err != nil {
			return nil, false, err
		}
		if !parentThread.Spec.Project {
			return nil, false, fmt.Errorf("parent thread %s is not a project", parentThreadName)
		}
		if !parentThread.Status.Created {
			return nil, false, nil
		}
		if parentThread.Status.WorkspaceName == "" {
			return nil, false, nil
		}
		result = append(result, parentThread.Status.WorkspaceName)
		parentThreadName = parentThread.Spec.ParentThreadName
	}

	if thread.Spec.AgentName != "" {
		var agent v1.Agent
		if err := c.Get(ctx, kclient.ObjectKey{Namespace: thread.Namespace, Name: thread.Spec.AgentName}, &agent); err != nil {
			return nil, false, err
		}
		if agent.Status.WorkspaceName == "" {
			// Waiting for the agent to be created
			return nil, false, nil
		}
		result = append(result, agent.Status.WorkspaceName)
	}

	slices.Reverse(result)
	return result, true, nil
}

func (t *Handler) CreateSharedWorkspace(req router.Request, _ router.Response) error {
	thread := req.Object.(*v1.Thread)
	if thread.Status.SharedWorkspaceName != "" || !thread.IsProjectBased() {
		return nil
	}

	var parentThread v1.Thread
	if thread.IsUserThread() {
		if err := req.Client.Get(req.Ctx, router.Key(thread.Namespace, thread.Spec.ParentThreadName), &parentThread); err != nil {
			return err
		}
		if parentThread.Status.SharedWorkspaceName == "" {
			// Wait to be created
			return nil
		}

		thread.Status.SharedWorkspaceName = parentThread.Status.SharedWorkspaceName
		return req.Client.Status().Update(req.Ctx, thread)
	}

	if !thread.IsProjectThread() {
		// this should never be hit
		panic("only project threads can create local workspace")
	}

	ws := v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    thread.Namespace,
			GenerateName: system.WorkspacePrefix,
			Finalizers:   []string{v1.WorkspaceFinalizer},
		},
		Spec: v1.WorkspaceSpec{
			ThreadName: thread.Name,
		},
	}

	if err := req.Client.Create(req.Ctx, &ws); err != nil {
		return err
	}

	thread.Status.SharedWorkspaceName = ws.Name
	return req.Client.Status().Update(req.Ctx, thread)
}

func getWorkspace(ctx context.Context, c kclient.WithWatch, thread *v1.Thread) (*v1.Workspace, error) {
	var ws v1.Workspace

	if thread.Spec.WorkspaceName != "" {
		return &ws, c.Get(ctx, kclient.ObjectKey{Namespace: thread.Namespace, Name: thread.Spec.WorkspaceName}, &ws)
	}

	if thread.Status.WorkspaceName != "" {
		return &ws, c.Get(ctx, kclient.ObjectKey{Namespace: thread.Namespace, Name: thread.Status.WorkspaceName}, &ws)
	}

	parents, ok, err := getParentWorkspaceNames(ctx, c, thread)
	if err != nil || !ok {
		return nil, err
	}

	ws = v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    thread.Namespace,
			GenerateName: system.WorkspacePrefix,
			Finalizers:   []string{v1.WorkspaceFinalizer},
		},
		Spec: v1.WorkspaceSpec{
			ThreadName:         thread.Name,
			FromWorkspaceNames: parents,
		},
	}

	return &ws, c.Create(ctx, &ws)
}

func (t *Handler) CreateWorkspaces(req router.Request, _ router.Response) error {
	thread := req.Object.(*v1.Thread)

	ws, err := getWorkspace(req.Ctx, req.Client, thread)
	if err != nil || ws == nil {
		return err
	}

	var update bool
	if thread.Status.WorkspaceID != ws.Status.WorkspaceID {
		update = true
		thread.Status.WorkspaceID = ws.Status.WorkspaceID
	}
	if thread.Status.WorkspaceName != ws.Name {
		update = true
		thread.Status.WorkspaceName = ws.Name
	}
	if update {
		return req.Client.Status().Update(req.Ctx, thread)
	}
	return nil
}

func createKnowledgeSet(ctx context.Context, c kclient.Client, thread *v1.Thread, relatedKnowledgeSets []string, from string) (*v1.KnowledgeSet, error) {
	var ks = v1.KnowledgeSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    thread.Namespace,
			GenerateName: system.KnowledgeSetPrefix,
			Finalizers:   []string{v1.KnowledgeSetFinalizer},
		},
		Spec: v1.KnowledgeSetSpec{
			ThreadName:               thread.Name,
			RelatedKnowledgeSetNames: relatedKnowledgeSets,
			FromKnowledgeSetName:     from,
		},
	}

	return &ks, c.Create(ctx, &ks)
}

func (t *Handler) CreateKnowledgeSet(req router.Request, _ router.Response) error {
	thread := req.Object.(*v1.Thread)
	if len(thread.Status.KnowledgeSetNames) > 0 || thread.Spec.AgentName == "" {
		return nil
	}

	var relatedKnowledgeSets []string
	var parentThreadName = thread.Spec.ParentThreadName

	if thread.Spec.SourceThreadName != "" {
		var sourceThread v1.Thread
		if err := req.Client.Get(req.Ctx, kclient.ObjectKey{Namespace: thread.Namespace, Name: thread.Spec.SourceThreadName}, &sourceThread); err != nil {
			return err
		}
		if sourceThread.Status.SharedKnowledgeSetName == "" {
			return nil
		}
		shared, err := createKnowledgeSet(req.Ctx, req.Client, thread, relatedKnowledgeSets, sourceThread.Status.SharedKnowledgeSetName)
		if err != nil {
			return err
		}

		thread.Status.SharedKnowledgeSetName = shared.Name
		if err := req.Client.Status().Update(req.Ctx, thread); err != nil {
			_ = req.Client.Delete(req.Ctx, shared)
			return err
		}
	} else {
		// Grab parents first so we have the list for the "related knowledge sets" if we need to create a new one
		for parentThreadName != "" {
			var parentThread v1.Thread
			if err := req.Client.Get(req.Ctx, kclient.ObjectKey{Namespace: thread.Namespace, Name: parentThreadName}, &parentThread); err != nil {
				return err
			}
			if !parentThread.Spec.Project {
				return fmt.Errorf("parent thread %s is not a project", parentThreadName)
			}
			if parentThread.Status.SharedKnowledgeSetName == "" {
				return nil
			}
			relatedKnowledgeSets = append(relatedKnowledgeSets, parentThread.Status.SharedKnowledgeSetName)
			parentThreadName = parentThread.Spec.ParentThreadName
		}
	}

	if thread.Status.SharedKnowledgeSetName == "" {
		shared, err := createKnowledgeSet(req.Ctx, req.Client, thread, relatedKnowledgeSets, "")
		if err != nil {
			return err
		}

		thread.Status.SharedKnowledgeSetName = shared.Name
		if err := req.Client.Status().Update(req.Ctx, thread); err != nil {
			_ = req.Client.Delete(req.Ctx, shared)
			return err
		}
	}

	relatedKnowledgeSets = append([]string{thread.Status.SharedKnowledgeSetName}, relatedKnowledgeSets...)
	thread.Status.KnowledgeSetNames = relatedKnowledgeSets
	return req.Client.Status().Update(req.Ctx, thread)
}

func (t *Handler) SetCreated(req router.Request, _ router.Response) error {
	thread := req.Object.(*v1.Thread)
	if thread.Status.Created {
		return nil
	}

	if thread.Status.WorkspaceID == "" {
		return nil
	}

	if thread.IsProjectBased() && thread.Status.SharedWorkspaceName == "" {
		return nil
	}

	if thread.Spec.SourceThreadName != "" && len(thread.Spec.Manifest.SharedTasks) > 0 && !thread.Status.CopiedTasks {
		return nil
	}

	if thread.Spec.SourceThreadName != "" && !thread.Status.CopiedTools {
		return nil
	}

	if thread.Spec.AgentName == "" {
		// Non-agent thread is ready at this point
		thread.Status.Created = true
		return req.Client.Status().Update(req.Ctx, thread)
	}

	if thread.Status.SharedKnowledgeSetName == "" {
		return nil
	}

	if len(thread.Status.KnowledgeSetNames) == 0 {
		return nil
	}

	thread.Status.Created = true
	return req.Client.Update(req.Ctx, thread)
}

// EnsureTemplateThreadShare ensures a public ThreadShare exists for template threads
func (t *Handler) EnsureTemplateThreadShare(req router.Request, _ router.Response) error {
	thread := req.Object.(*v1.Thread)
	if !thread.Spec.Template {
		return nil
	}
	// Create the share if it doesn't exist
	var share v1.ThreadShare
	if err := req.Client.Get(req.Ctx, router.Key(thread.Namespace, thread.Name), &share); err == nil {
		return nil
	} else if !apierrors.IsNotFound(err) {
		return err
	}

	publicID := strings.ReplaceAll(uuid.New().String(), "-", "")
	share = v1.ThreadShare{
		ObjectMeta: metav1.ObjectMeta{
			Name:      thread.Name,
			Namespace: thread.Namespace,
		},
		Spec: v1.ThreadShareSpec{
			UserID:            thread.Spec.UserID,
			ProjectThreadName: thread.Name,
			Template:          true,
			Featured:          false,
			Manifest:          types.ProjectShareManifest{Public: true},
			PublicID:          publicID,
		},
	}
	return req.Client.Create(req.Ctx, &share)
}

func (t *Handler) CleanupEphemeralThreads(req router.Request, _ router.Response) error {
	thread := req.Object.(*v1.Thread)
	if !thread.Spec.Ephemeral ||
		thread.CreationTimestamp.After(time.Now().Add(-12*time.Hour)) {
		return nil
	}

	return kclient.IgnoreNotFound(req.Delete(thread))
}

func (t *Handler) GenerateName(req router.Request, _ router.Response) error {
	thread := req.Object.(*v1.Thread)
	if !thread.IsUserThread() || thread.Spec.Manifest.Name != "" || thread.Status.LastRunName == "" ||
		thread.Spec.Ephemeral ||
		thread.Status.LastRunState != v1.Continue && thread.Status.LastRunState != v1.Waiting {
		return nil
	}

	var run v1.Run
	if err := req.Get(&run, thread.Namespace, thread.Status.LastRunName); err != nil {
		return err
	}

	result, err := t.invoker.EphemeralThreadTask(req.Ctx, thread, gptscript.ToolDef{
		Instructions: `Generate a concise (3 to 4 words) and descriptive thread name that encapsulates the main topic or theme of the following conversation starter. Do not enclose the title in quotes.`,
	}, fmt.Sprintf("User Input: %s\n\nLLM Response: %s", run.Spec.Input, run.Status.Output))
	if err != nil {
		return fmt.Errorf("failed to generate thread name: %w", err)
	}

	thread.Spec.Manifest.Name = strings.TrimSpace(result)
	return req.Client.Update(req.Ctx, thread)
}

func (t *Handler) EnsureShared(req router.Request, _ router.Response) error {
	wf := req.Object.(*v1.Workflow)
	if !wf.Spec.Managed {
		return nil
	}

	var sourceThread v1.Thread
	if err := req.Get(&sourceThread, wf.Namespace, wf.Spec.SourceThreadName); apierrors.IsNotFound(err) {
		return req.Delete(wf)
	} else if err != nil {
		return fmt.Errorf("failed to get source thread %s: %w", wf.Spec.SourceThreadName, err)
	}

	if !slices.Contains(sourceThread.Spec.Manifest.SharedTasks, wf.Spec.SourceWorkflowName) {
		return req.Delete(wf)
	}

	return nil
}

func (t *Handler) CopyTasksFromSource(req router.Request, _ router.Response) error {
	thread := req.Object.(*v1.Thread)
	if !thread.Spec.Project || thread.Spec.SourceThreadName == "" || thread.Spec.ParentThreadName != "" {
		return nil
	}

	if thread.Status.CopiedTasks {
		return nil
	}

	// Delete all existing tasks for this thread
	var existing v1.WorkflowList
	if err := req.Client.List(req.Ctx, &existing, kclient.InNamespace(thread.Namespace), kclient.MatchingFields{
		"spec.threadName": thread.Name,
	}); err != nil {
		return err
	}
	for _, wf := range existing.Items {
		if err := req.Client.Delete(req.Ctx, &wf); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}

	// Copy all tasks from the source thread to the new thread
	var srcList v1.WorkflowList
	if err := req.Client.List(req.Ctx, &srcList, kclient.InNamespace(thread.Namespace), kclient.MatchingFields{
		"spec.threadName": thread.Spec.SourceThreadName,
	}); err != nil {
		return err
	}
	for _, src := range srcList.Items {
		newManifest := src.Spec.Manifest
		alias, err := randomtoken.Generate()
		if err != nil {
			return err
		}
		newManifest.Alias = alias
		wf := v1.Workflow{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: system.WorkflowPrefix,
				Namespace:    thread.Namespace,
			},
			Spec: v1.WorkflowSpec{
				ThreadName: thread.Name,
				Manifest:   newManifest,
			},
		}
		if err := req.Client.Create(req.Ctx, &wf); err != nil {
			return err
		}
	}

	thread.Status.CopiedTasks = true
	return req.Client.Status().Update(req.Ctx, thread)
}

func (t *Handler) CopyToolsFromSource(req router.Request, _ router.Response) error {
	thread := req.Object.(*v1.Thread)
	if !thread.Spec.Project || thread.Spec.SourceThreadName == "" || thread.Spec.ParentThreadName != "" {
		return nil
	}

	if thread.Status.CopiedTools {
		return nil
	}

	var toolList v1.ToolList
	if err := req.Client.List(req.Ctx, &toolList, kclient.InNamespace(thread.Namespace), kclient.MatchingFields{
		"spec.threadName": thread.Spec.SourceThreadName,
	}); err != nil {
		return err
	}

	for _, tool := range toolList.Items {
		newTool := v1.Tool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name.SafeHashConcatName(tool.Name, thread.Name),
				Namespace: thread.Namespace,
			},
			Spec: v1.ToolSpec{
				ThreadName: thread.Name,
				Manifest:   tool.Spec.Manifest,
			},
		}
		if err := create.IfNotExists(req.Ctx, req.Client, &newTool); err != nil {
			return err
		}
	}

	if err := t.copyMCPServersFromSource(req, thread); err != nil {
		return err
	}

	thread.Status.CopiedTools = true
	return req.Client.Status().Update(req.Ctx, thread)
}

func (t *Handler) RemoveOldFinalizers(req router.Request, _ router.Response) error {
	thread := req.Object.(*v1.Thread)

	finalizerCount := len(thread.Finalizers)
	thread.Finalizers = slices.DeleteFunc(thread.Finalizers, func(finalizer string) bool {
		return finalizer == v1.ThreadFinalizer+"-child-cleanup" || finalizer == v1.MCPServerFinalizer
	})

	if finalizerCount != len(thread.Finalizers) {
		return req.Client.Update(req.Ctx, thread)
	}
	return nil
}

func (t *Handler) copyMCPServersFromSource(req router.Request, thread *v1.Thread) error {
	var sourceProjectMCPList v1.ProjectMCPServerList
	if err := req.Client.List(req.Ctx, &sourceProjectMCPList, kclient.InNamespace(thread.Namespace), kclient.MatchingFields{
		"spec.threadName": thread.Spec.SourceThreadName,
	}); err != nil {
		return err
	}

	desiredProjectMCPServers := make(map[string]v1.ProjectMCPServer, len(sourceProjectMCPList.Items))
	for _, sourcePMS := range sourceProjectMCPList.Items {
		var (
			copiedPMS *v1.ProjectMCPServer
			err       error
		)
		if system.IsMCPServerInstanceID(sourcePMS.Spec.Manifest.MCPID) {
			// Handle multi-user MCP servers (MCPServerInstance)
			copiedPMS, err = t.copyMCPServerInstance(req, &sourcePMS, thread)
		} else {
			// Handle single-user or remote MCP servers (MCPServer)
			copiedPMS, err = t.copyMCPServer(req, &sourcePMS, thread)
		}
		if err != nil {
			return err
		}

		if copiedPMS == nil {
			// This should never happen if copying MCP servers/instances returns no error
			continue
		}

		desiredProjectMCPServers[copiedPMS.Name] = *copiedPMS
	}

	// Handle ProjectMCPServer creation/updates in a single operation
	for pmsName, desiredPMS := range desiredProjectMCPServers {
		var existingPMS v1.ProjectMCPServer
		if err := req.Get(&existingPMS, thread.Namespace, pmsName); err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}

			// ProjectMCPServer doesn't exist, create it
			if err := req.Client.Create(req.Ctx, &desiredPMS); err != nil {
				return err
			}
		} else {
			// ProjectMCPServer exists, update it
			existingPMS.Spec = desiredPMS.Spec
			if err := req.Client.Update(req.Ctx, &existingPMS); err != nil {
				return err
			}
		}
	}

	// Prune ProjectMCPServers that are no longer desired for this thread
	var existingPMSList v1.ProjectMCPServerList
	if err := req.Client.List(req.Ctx, &existingPMSList, kclient.InNamespace(thread.Namespace), kclient.MatchingFields{
		"spec.threadName": thread.Name,
	}); err != nil {
		return err
	}
	for _, pms := range existingPMSList.Items {
		if _, keep := desiredProjectMCPServers[pms.Name]; !keep {
			if err := kclient.IgnoreNotFound(req.Client.Delete(req.Ctx, &pms)); err != nil {
				return err
			}
		}
	}

	return nil
}

// copyMCPServerInstance copies MCP servers from the source thread to the destination thread,
// creating appropriate per-user instances and project mappings.
// copyMultiUserMCPServer handles copying multi-user MCP servers (MCPServerInstance)
func (*Handler) copyMCPServerInstance(req router.Request, sourcePMS *v1.ProjectMCPServer, thread *v1.Thread) (*v1.ProjectMCPServer, error) {
	sourceMCPID := sourcePMS.Spec.Manifest.MCPID

	// Get the source MCPServerInstance
	var sourceMCPServerInstance v1.MCPServerInstance
	if err := req.Get(&sourceMCPServerInstance, thread.Namespace, sourceMCPID); err != nil {
		return nil, err
	}

	// Create or update the copied MCPServerInstance
	var (
		copiedMCPInstanceID = name.SafeHashConcatName(sourceMCPID, thread.Name)
		copiedMCPInstance   v1.MCPServerInstance
	)
	if err := req.Get(&copiedMCPInstance, thread.Namespace, copiedMCPInstanceID); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}

		// We didn't find a copied MCP server instance for the user, so create a new one
		copiedMCPInstance = v1.MCPServerInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      copiedMCPInstanceID,
				Namespace: thread.Namespace,
			},
			Spec: v1.MCPServerInstanceSpec{
				MCPServerName:             sourceMCPServerInstance.Spec.MCPServerName,
				MCPCatalogName:            sourceMCPServerInstance.Spec.MCPCatalogName,
				MCPServerCatalogEntryName: sourceMCPServerInstance.Spec.MCPServerCatalogEntryName,
				PowerUserWorkspaceID:      sourceMCPServerInstance.Spec.PowerUserWorkspaceID,
				UserID:                    thread.Spec.UserID,
				Template:                  thread.Spec.Template,
			},
		}

		if err := req.Client.Create(req.Ctx, &copiedMCPInstance); err != nil {
			return nil, err
		}
	} else {
		// We found an existing copied MCP server instance, update it
		copiedMCPInstance.Spec.MCPServerName = sourceMCPServerInstance.Spec.MCPServerName
		copiedMCPInstance.Spec.MCPCatalogName = sourceMCPServerInstance.Spec.MCPCatalogName
		copiedMCPInstance.Spec.MCPServerCatalogEntryName = sourceMCPServerInstance.Spec.MCPServerCatalogEntryName
		copiedMCPInstance.Spec.PowerUserWorkspaceID = sourceMCPServerInstance.Spec.PowerUserWorkspaceID
		copiedMCPInstance.Spec.UserID = thread.Spec.UserID
		copiedMCPInstance.Spec.Template = thread.Spec.Template

		if err := req.Client.Update(req.Ctx, &copiedMCPInstance); err != nil {
			return nil, err
		}
	}

	// Return the desired ProjectMCPServer
	copiedPMSName := name.SafeHashConcatName(sourcePMS.Name, thread.Name)
	return &v1.ProjectMCPServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:       copiedPMSName,
			Namespace:  thread.Namespace,
			Finalizers: []string{v1.ProjectMCPServerFinalizer},
		},
		Spec: v1.ProjectMCPServerSpec{
			Manifest: types.ProjectMCPServerManifest{
				MCPID: copiedMCPInstanceID,
				Alias: sourcePMS.Spec.Manifest.Alias,
			},
			ThreadName: thread.Name,
			UserID:     thread.Spec.UserID,
		},
	}, nil
}

// copyMCPServer handles copying single-user or remote MCP servers (MCPServer)
func (*Handler) copyMCPServer(req router.Request, sourcePMS *v1.ProjectMCPServer, thread *v1.Thread) (*v1.ProjectMCPServer, error) {
	sourceMCPID := sourcePMS.Spec.Manifest.MCPID

	// Get the source MCPServer
	var sourceMCPServer v1.MCPServer
	if err := req.Get(&sourceMCPServer, thread.Namespace, sourceMCPID); err != nil {
		return nil, err
	}

	// Create or update the copied MCPServer
	var (
		copiedMCPID     = name.SafeHashConcatName(sourceMCPID, thread.Name)
		copiedMCPServer v1.MCPServer
	)
	if err := req.Get(&copiedMCPServer, thread.Namespace, copiedMCPID); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}

		// We didn't find a copied MCP server for the user, so create a new one
		copiedMCPServer = v1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{
				Name:       copiedMCPID,
				Namespace:  thread.Namespace,
				Finalizers: []string{v1.MCPServerFinalizer},
			},
			Spec: v1.MCPServerSpec{
				Manifest:                  sourceMCPServer.Spec.Manifest,
				UnsupportedTools:          sourceMCPServer.Spec.UnsupportedTools,
				Alias:                     sourceMCPServer.Spec.Alias,
				UserID:                    thread.Spec.UserID,
				MCPServerCatalogEntryName: sourceMCPServer.Spec.MCPServerCatalogEntryName,
				MCPCatalogID:              sourceMCPServer.Spec.MCPCatalogID,
				PowerUserWorkspaceID:      sourceMCPServer.Spec.PowerUserWorkspaceID,
				Template:                  thread.Spec.Template,
			},
		}

		if err := req.Client.Create(req.Ctx, &copiedMCPServer); err != nil {
			return nil, err
		}
	} else {
		// We found an existing copied MCP server, update it
		copiedMCPServer.Spec.Manifest = sourceMCPServer.Spec.Manifest
		copiedMCPServer.Spec.UnsupportedTools = sourceMCPServer.Spec.UnsupportedTools
		copiedMCPServer.Spec.Alias = sourceMCPServer.Spec.Alias
		copiedMCPServer.Spec.UserID = thread.Spec.UserID
		copiedMCPServer.Spec.MCPServerCatalogEntryName = sourceMCPServer.Spec.MCPServerCatalogEntryName
		copiedMCPServer.Spec.MCPCatalogID = sourceMCPServer.Spec.MCPCatalogID
		copiedMCPServer.Spec.PowerUserWorkspaceID = sourceMCPServer.Spec.PowerUserWorkspaceID
		copiedMCPServer.Spec.Template = thread.Spec.Template

		if err := req.Client.Update(req.Ctx, &copiedMCPServer); err != nil {
			return nil, err
		}
	}

	// Return the desired ProjectMCPServer
	copiedPMSName := name.SafeHashConcatName(sourcePMS.Name, thread.Name)
	return &v1.ProjectMCPServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:       copiedPMSName,
			Namespace:  thread.Namespace,
			Finalizers: []string{v1.ProjectMCPServerFinalizer},
		},
		Spec: v1.ProjectMCPServerSpec{
			Manifest: types.ProjectMCPServerManifest{
				MCPID: copiedMCPID,
				Alias: sourceMCPServer.Spec.Alias,
			},
			ThreadName: thread.Name,
			UserID:     thread.Spec.UserID,
		},
	}, nil
}

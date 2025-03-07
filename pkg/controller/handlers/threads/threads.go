package threads

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/nah/pkg/randomtoken"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/nah/pkg/untriggered"
	"github.com/obot-platform/obot/pkg/invoke"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
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
		// Projects don't copy the parents
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

func (t *Handler) CreateLocalWorkspace(req router.Request, _ router.Response) error {
	thread := req.Object.(*v1.Thread)
	if thread.Status.LocalWorkspaceName != "" || !thread.IsProjectBased() {
		return nil
	}

	var (
		parentThread       v1.Thread
		fromWorkspaceNames []string
	)

	if thread.Spec.ParentThreadName != "" {
		if err := req.Client.Get(req.Ctx, router.Key(thread.Namespace, thread.Spec.ParentThreadName), &parentThread); err != nil {
			return err
		}
		if parentThread.Status.LocalWorkspaceName == "" {
			// Wait to be created
			return nil
		}
		fromWorkspaceNames = append(fromWorkspaceNames, parentThread.Status.LocalWorkspaceName)
	}

	if thread.IsUserThread() {
		thread.Status.LocalWorkspaceName = parentThread.Status.LocalWorkspaceName
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
			ThreadName:         thread.Name,
			FromWorkspaceNames: fromWorkspaceNames,
		},
	}

	if err := req.Client.Create(req.Ctx, &ws); err != nil {
		return err
	}

	thread.Status.LocalWorkspaceName = ws.Name
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

func createKnowledgeSet(ctx context.Context, c kclient.Client, thread *v1.Thread, relatedKnowledgeSets []string) (*v1.KnowledgeSet, error) {
	var ks = v1.KnowledgeSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    thread.Namespace,
			GenerateName: system.KnowledgeSetPrefix,
			Finalizers:   []string{v1.KnowledgeSetFinalizer},
		},
		Spec: v1.KnowledgeSetSpec{
			ThreadName:               thread.Name,
			RelatedKnowledgeSetNames: relatedKnowledgeSets,
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

	if thread.Status.SharedKnowledgeSetName == "" {
		shared, err := createKnowledgeSet(req.Ctx, req.Client, thread, relatedKnowledgeSets)
		if err != nil {
			_ = req.Client.Delete(req.Ctx, shared)
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

	if thread.IsProjectBased() && thread.Status.LocalWorkspaceName == "" {
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
	if !thread.IsUserThread() || thread.Spec.Manifest.Name != "" || thread.Status.LastRunName == "" || thread.Status.LastRunState != gptscript.Continue {
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

func (t *Handler) ActivateRuns(req router.Request, _ router.Response) error {
	var runs v1.RunList
	// This must be uncached since inactive things aren't in the cache.
	if err := req.List(untriggered.UncachedList(&runs), &kclient.ListOptions{
		Namespace:     req.Namespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{"spec.threadName": req.Object.GetName()}),
	}); err != nil {
		return fmt.Errorf("failed to list runs for thread %s: %w", req.Object.GetName(), err)
	}

	for _, run := range runs.Items {
		if !v1.IsActive(&run) {
			v1.SetActive(&run)
			if err := req.Client.Update(req.Ctx, &run); err != nil {
				return fmt.Errorf("failed to update run %q to active: %w", run.Name, err)
			}
		}
	}

	return nil
}

func (t *Handler) EnsureShared(req router.Request, _ router.Response) error {
	wf := req.Object.(*v1.Workflow)
	if !wf.Spec.ProjectScoped {
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

func (t *Handler) CopyTasks(req router.Request, _ router.Response) error {
	thread := req.Object.(*v1.Thread)
	if !thread.Spec.Project || thread.Spec.ParentThreadName == "" {
		return nil
	}

	var parentThread v1.Thread
	if err := req.Get(&parentThread, thread.Namespace, thread.Spec.ParentThreadName); apierrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to get parent thread %s: %w", thread.Spec.ParentThreadName, err)
	}

	for _, taskID := range parentThread.Spec.Manifest.SharedTasks {
		var wf v1.Workflow
		if err := req.Get(&wf, thread.Namespace, taskID); apierrors.IsNotFound(err) {
			continue
		} else if err != nil {
			return fmt.Errorf("failed to get workflow %s: %w", taskID, err)
		} else if wf.Spec.ThreadName != parentThread.Name {
			continue
		}

		var (
			targetWFName = name.SafeHashConcatName(wf.Name, thread.Name)
			targetWF     v1.Workflow
			newManifest  = wf.Spec.Manifest
		)
		if err := req.Get(&targetWF, thread.Namespace, targetWFName); apierrors.IsNotFound(err) {
			newManifest.Alias, err = randomtoken.Generate()
			if err != nil {
				return err
			}

			err := req.Client.Create(req.Ctx, &v1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      targetWFName,
					Namespace: thread.Namespace,
				},
				Spec: v1.WorkflowSpec{
					ThreadName:         thread.Name,
					Manifest:           newManifest,
					ProjectScoped:      true,
					SourceThreadName:   parentThread.Name,
					SourceWorkflowName: wf.Name,
				},
			})
			if err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			newManifest.Alias = targetWF.Spec.Manifest.Alias
			if !equality.Semantic.DeepEqual(targetWF.Spec.Manifest, newManifest) {
				targetWF.Spec.Manifest = newManifest
				if err := req.Client.Update(req.Ctx, &targetWF); err != nil {
					return fmt.Errorf("failed to update workflow %s: %w", targetWF.Name, err)
				}
			}
		}
	}

	return nil
}

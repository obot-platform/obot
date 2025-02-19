package threadtemplate

import (
	"strings"

	"github.com/google/uuid"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/api/handlers"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateTemplate(req router.Request, _ router.Response) error {
	template := req.Object.(*v1.ThreadTemplate)
	if template.Status.Ready {
		return nil
	}

	var parentThread v1.Thread
	if err := req.Client.Get(req.Ctx, router.Key(template.Namespace, template.Spec.ProjectThreadName), &parentThread); apierrors.IsNotFound(err) {
		return req.Delete(template)
	} else if err != nil {
		return err
	}

	if err := createWorkspace(req, &parentThread, template); err != nil {
		return err
	}

	if err := createKnowledgeWorkspace(req, &parentThread, template); err != nil {
		return err
	}

	var workflows v1.WorkflowList
	if err := req.Client.List(req.Ctx, &workflows, kclient.InNamespace(template.Namespace), kclient.MatchingFields{
		"spec.threadName": template.Name,
	}); err != nil {
		return err
	}

	template.Status.Tasks = nil
	for _, workflow := range workflows.Items {
		template.Status.Tasks = append(template.Status.Tasks, handlers.ConvertTaskManifest(&workflow.Spec.Manifest))
	}

	template.Status.Manifest = parentThread.Spec.Manifest
	template.Status.AgentName = parentThread.Spec.AgentName
	template.Status.PublicID = strings.ReplaceAll(uuid.New().String(), "-", "")
	template.Status.Ready = true

	return req.Client.Status().Update(req.Ctx, template)
}

func createKnowledgeWorkspace(req router.Request, parentThread *v1.Thread, template *v1.ThreadTemplate) error {
	if template.Status.KnowledgeWorkspaceName != "" {
		return nil
	}

	if len(parentThread.Status.KnowledgeSetNames) != 1 {
		return nil
	}

	var knowledgeSet v1.KnowledgeSet
	if err := req.Client.Get(req.Ctx, router.Key(template.Namespace, parentThread.Status.KnowledgeSetNames[0]), &knowledgeSet); err != nil {
		return err
	}

	var workspace v1.Workspace
	if err := req.Client.Get(req.Ctx, router.Key(template.Namespace, knowledgeSet.Status.WorkspaceName), &workspace); err != nil {
		return err
	}

	newWorkspace := v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.WorkspacePrefix,
			Namespace:    template.Namespace,
		},
		Spec: v1.WorkspaceSpec{
			ThreadTemplateName: template.Name,
			FromWorkspaceNames: []string{workspace.Name},
		},
	}

	if err := req.Client.Create(req.Ctx, &newWorkspace); err != nil {
		return err
	}

	template.Status.KnowledgeWorkspaceName = newWorkspace.Name
	return req.Client.Status().Update(req.Ctx, template)
}

func createWorkspace(req router.Request, parentThread *v1.Thread, template *v1.ThreadTemplate) error {
	if template.Status.WorkspaceName != "" {
		return nil
	}

	var workspace v1.Workspace
	if err := req.Client.Get(req.Ctx, router.Key(template.Namespace, parentThread.Status.WorkspaceName), &workspace); err != nil {
		return err
	}

	newWorkspace := v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.WorkspacePrefix,
			Namespace:    template.Namespace,
		},
		Spec: v1.WorkspaceSpec{
			ThreadTemplateName: template.Name,
			FromWorkspaceNames: []string{workspace.Name},
		},
	}

	if err := req.Client.Create(req.Ctx, &newWorkspace); err != nil {
		return err
	}

	template.Status.WorkspaceName = newWorkspace.Name
	return req.Client.Status().Update(req.Ctx, template)
}

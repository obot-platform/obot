package knowledgeset

import (
	"context"
	"fmt"

	"github.com/obot-platform/nah/pkg/name"
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
	invoker *invoke.Invoker
}

func New(invoker *invoke.Invoker) *Handler {
	return &Handler{
		invoker: invoker,
	}
}

func createWorkspace(ctx context.Context, c kclient.Client, ks *v1.KnowledgeSet) error {
	if ks.Status.WorkspaceName != "" {
		return nil
	}

	ws := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name.SafeConcatName(system.WorkspacePrefix, ks.Name),
			Namespace:  ks.Namespace,
			Finalizers: []string{v1.WorkspaceFinalizer},
		},
		Spec: v1.WorkspaceSpec{
			KnowledgeSetName: ks.Name,
		},
	}
	err := create.OrGet(ctx, c, ws)
	if err != nil {
		return err
	}

	ks.Status.WorkspaceName = ws.Name
	return c.Status().Update(ctx, ks)
}

func (h *Handler) createThread(ctx context.Context, c kclient.Client, ks *v1.KnowledgeSet) error {
	thread := &v1.Thread{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.SafeConcatName(system.ThreadPrefix, ks.Name),
			Namespace: ks.Namespace,
		},
		Spec: v1.ThreadSpec{
			KnowledgeSetName: ks.Name,
			WorkspaceName:    ks.Status.WorkspaceName,
			SystemTask:       true,
		},
	}
	// Threads are special because we assume users might delete them randomly
	err := create.IfNotExists(ctx, c, thread)
	if err != nil {
		return err
	}

	if ks.Status.ThreadName == "" {
		ks.Status.ThreadName = thread.Name
		return c.Status().Update(ctx, ks)
	}
	return nil
}

func (h *Handler) CheckHasContent(req router.Request, _ router.Response) error {
	ks := req.Object.(*v1.KnowledgeSet)

	// This is a hack to track exactly when the knowledge set has no more content.
	// The issue is triggers. Triggers on field or label selectors work fine, but not for deleted objects.
	// When an object is deleted, there is no way to tell if it matches the field selector because the object is gone.
	// Therefore, field and label selector triggers don't trigger on deletion.
	// However, it is important that we clean up the dataset when the knowledge set is empty.
	// So, we track a single file because this will be triggered when the file is deleted. Once the last file is deleted, then the knowledge set is empty,
	// and we can clean up the dataset.
	if ks.Status.ExistingFile != "" {
		var file v1.KnowledgeFile
		if err := req.Get(&file, req.Namespace, ks.Status.ExistingFile); err == nil {
			return nil
		} else if !apierrors.IsNotFound(err) {
			return err
		}
	}

	var files v1.KnowledgeFileList
	if err := req.Client.List(req.Ctx, &files, kclient.InNamespace(ks.Namespace), kclient.MatchingFields{
		"spec.knowledgeSetName": ks.Name,
	}); err != nil {
		return err
	}

	ks.Status.HasContent = len(files.Items) > 0
	if !ks.Status.HasContent {
		// Reset the embedding model so it can be implicitly updated when knowledge is added.
		ks.Status.TextEmbeddingModel = ""
		ks.Status.ExistingFile = ""
	} else {
		ks.Status.ExistingFile = files.Items[0].Name
		ks.Status.DatasetCreated = true
	}

	return nil
}

func (h *Handler) SetEmbeddingModel(req router.Request, _ router.Response) error {
	ks := req.Object.(*v1.KnowledgeSet)
	if !ks.Status.HasContent || ks.Status.TextEmbeddingModel != "" {
		return nil
	}

	if ks.Spec.TextEmbeddingModel != "" {
		ks.Status.TextEmbeddingModel = ks.Spec.TextEmbeddingModel
		return nil
	}

	var defaultEmbeddingModel v1.DefaultModelAlias
	if err := req.Get(&defaultEmbeddingModel, req.Namespace, string(types.DefaultModelAliasTypeTextEmbedding)); err != nil {
		return err
	}

	ks.Status.TextEmbeddingModel = defaultEmbeddingModel.Spec.Manifest.Model

	return nil
}

func (h *Handler) CreateWorkspace(req router.Request, _ router.Response) error {
	ks := req.Object.(*v1.KnowledgeSet)

	if err := createWorkspace(req.Ctx, req.Client, ks); err != nil {
		return err
	}

	return h.createThread(req.Ctx, req.Client, ks)
}

func (h *Handler) Cleanup(req router.Request, _ router.Response) error {
	ks := req.Object.(*v1.KnowledgeSet)
	if ks.Status.ThreadName == "" || !ks.Status.DatasetCreated || (ks.DeletionTimestamp.IsZero() && ks.Status.HasContent) {
		return nil
	}

	var thread v1.Thread
	if err := req.Client.Get(req.Ctx, router.Key(ks.Namespace, ks.Status.ThreadName), &thread); apierrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return err
	}

	task, err := h.invoker.SystemTask(req.Ctx, &thread, system.KnowledgeDeleteTool, ks.Namespace+"/"+ks.Name)
	if err != nil {
		return err
	}
	defer task.Close()

	_, err = task.Result(req.Ctx)
	if err != nil {
		return fmt.Errorf("failed to delete knowledge set: %w", err)
	}

	ks.Status.DatasetCreated = false
	return nil
}

package knowledgesource

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/acorn-io/baaah/pkg/apply"
	"github.com/acorn-io/baaah/pkg/name"
	"github.com/acorn-io/baaah/pkg/router"
	"github.com/gptscript-ai/go-gptscript"
	"github.com/otto8-ai/otto8/apiclient/types"
	"github.com/otto8-ai/otto8/logger"
	"github.com/otto8-ai/otto8/pkg/create"
	"github.com/otto8-ai/otto8/pkg/gz"
	"github.com/otto8-ai/otto8/pkg/invoke"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.gptscript.ai/v1"
	"github.com/otto8-ai/otto8/pkg/system"
	"github.com/otto8-ai/otto8/pkg/wait"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var log = logger.Package()

type Handler struct {
	invoker   *invoke.Invoker
	gptClient *gptscript.GPTScript
}

func NewHandler(invoker *invoke.Invoker, gptClient *gptscript.GPTScript) *Handler {
	return &Handler{
		invoker:   invoker,
		gptClient: gptClient,
	}
}

func shouldRerun(source *v1.KnowledgeSource) bool {
	return source.Spec.SyncGeneration > source.Status.SyncGeneration ||
		source.Status.SyncState == types.KnowledgeSourceStatePending
}

func safeStatusSave(ctx context.Context, c kclient.Client, source *v1.KnowledgeSource) (err error) {
	// This logic is done mostly because a sync is a very long operation so a 409 is super impactful because it could
	// force a restart. Where other thing we don't care so much if we have to restart, but restarting a 20 minute long
	// thing really sucks
	status := source.Status.DeepCopy()
	for i := 0; i < 20; i++ {
		if err = c.Status().Update(ctx, source); apierror.IsConflict(err) {
			time.Sleep(500 * time.Millisecond)
			if err := c.Get(ctx, kclient.ObjectKeyFromObject(source), source); err != nil {
				return err
			}
			// restore full status we wanted to save
			source.Status = *status
			continue
		} else if err != nil {
			return err
		}
		return nil
	}

	// This should be the error from the last loop, which should be a conflict
	return err
}
func (k *Handler) saveProgress(ctx context.Context, c kclient.Client, source *v1.KnowledgeSource, thread *v1.Thread, complete bool) error {
	files, syncMetadata, err := k.getMetadata(ctx, source, thread)
	if err != nil {
		return err
	}
	apply := apply.New(c)
	if !complete {
		apply = apply.WithNoPrune()
	}
	if err := apply.Apply(ctx, source, files...); err != nil {
		return err
	}

	syncDetails, err := gz.Compress(syncMetadata.State)
	if err != nil {
		return err
	}

	if syncMetadata.Status != source.Status.Status ||
		!bytes.Equal(syncDetails, source.Status.SyncDetails) {
		source.Status.Status = syncMetadata.Status
		source.Status.SyncDetails = syncDetails
		if err := safeStatusSave(ctx, c, source); err != nil {
			return err
		}
	}

	return nil
}

func getThread(ctx context.Context, c kclient.WithWatch, source *v1.KnowledgeSource) (*v1.Thread, error) {
	var update bool

	if source.Status.WorkspaceName == "" {
		ws := &v1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name.SafeConcatName(system.WorkspacePrefix, source.Name),
				Namespace: source.Namespace,
			},
			Spec: v1.WorkspaceSpec{
				KnowledgeSourceName: source.Name,
			},
		}
		if err := create.OrGet(ctx, c, ws); err != nil {
			return nil, err
		}

		source.Status.WorkspaceName = ws.Name
		// We don't update immediately because the name is deterministic so we can save one update
		update = true
	}

	thread := &v1.Thread{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.SafeConcatName(system.ThreadPrefix, source.Name),
			Namespace: source.Namespace,
		},
		Spec: v1.ThreadSpec{
			KnowledgeSourceName: source.Name,
			WorkspaceName:       source.Status.WorkspaceName,
			SystemTask:          true,
		},
	}
	// Threads are special because we assume users might delete them randomly
	if err := create.IfNotExists(ctx, c, thread); err != nil {
		return nil, err
	}

	if source.Status.ThreadName == "" {
		source.Status.ThreadName = thread.Name
		update = true
	}

	if update {
		if err := c.Status().Update(ctx, source); err != nil {
			return nil, err
		}
	}

	return wait.For(ctx, c, thread, func(thread *v1.Thread) bool {
		return thread.Status.WorkspaceID != ""
	})
}

func (k *Handler) Sync(req router.Request, _ router.Response) error {
	source := req.Object.(*v1.KnowledgeSource)

	if source.Status.Auth.Required == nil || (*source.Status.Auth.Required && !source.Status.Auth.Authenticated) {
		return nil
	}

	invokeOpts := invoke.SystemTaskOptions{
		CredentialContextIDs: []string{source.Name},
	}

	thread, err := getThread(req.Ctx, req.Client, source)
	if err != nil {
		return err
	}

	if source.Status.SyncState == types.KnowledgeSourceStateSyncing {
		// We are recovering from a system restart, go back to pending and re-evaluate,
		source.Status.SyncState = types.KnowledgeSourceStatePending
	}

	if source.Status.SyncState.IsTerminal() && !shouldRerun(source) {
		return nil
	}

	sourceType := source.Spec.Manifest.GetType()
	if sourceType == "" {
		source.Status.Error = "unknown knowledge source type"
		source.Status.SyncState = types.KnowledgeSourceStateError
		return req.Client.Status().Update(req.Ctx, source)
	}

	task, err := k.invoker.SystemTask(req.Ctx, thread, string(sourceType)+"-data-source", source.Spec.Manifest.KnowledgeSourceInput, invokeOpts)
	if err != nil {
		return err
	}
	defer task.Close()

	source.Status.LastSyncStartTime = metav1.Now()
	source.Status.LastSyncEndTime = metav1.Time{}
	source.Status.NextSyncTime = metav1.Time{}
	source.Status.SyncState = types.KnowledgeSourceStateSyncing
	source.Status.ThreadName = task.Thread.Name
	source.Status.RunName = task.Run.Name
	if err := req.Client.Status().Update(req.Ctx, source); err != nil {
		return err
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

forLoop:
	for {
		select {
		case _, ok := <-task.Events:
			if !ok {
				// done
				break forLoop
			}
		case <-ticker.C:
			if err := k.saveProgress(req.Ctx, req.Client, source, thread, false); err != nil {
				// Ignore these errors, hopefully transient
				log.Errorf("failed to get files for knowledgesource [%s]: %v", source.Name, err)
			}
		}
	}

	taskResult, err := task.Result(req.Ctx)
	if err != nil {
		return err
	}

	if err := k.saveProgress(req.Ctx, req.Client, source, thread, taskResult.Error == ""); err != nil {
		log.Errorf("failed to save files for knowledgesource [%s]: %v", source.Name, err)
		if taskResult.Error == "" {
			taskResult.Error = err.Error()
		}
	}

	source.Status.LastSyncEndTime = metav1.Now()
	source.Status.SyncGeneration = source.Spec.SyncGeneration
	source.Status.RunName = ""
	source.Status.Error = taskResult.Error
	if taskResult.Error == "" {
		source.Status.SyncState = types.KnowledgeSourceStateSynced
	} else {
		source.Status.SyncState = types.KnowledgeSourceStateError
	}
	return safeStatusSave(req.Ctx, req.Client, source)
}

func (k *Handler) BackPopulateAuthStatus(req router.Request, _ router.Response) error {
	source := req.Object.(*v1.KnowledgeSource)
	if source.Status.Auth.Authenticated || (source.Status.Auth.Required != nil && !*source.Status.Auth.Required) {
		return nil
	}

	_, required, err := source.CredentialTool(req.Ctx, req.Client)
	if err != nil {
		return fmt.Errorf("failed to get credential tool for knowledge source [%s]: %w", source.Name, err)
	}
	if !required {
		source.Status.Auth = types.OAuthAppLoginAuthStatus{
			Required: &required,
		}
		return req.Client.Status().Update(req.Ctx, source)
	}

	var oauthAppLogin v1.OAuthAppLogin
	if err := req.Get(&oauthAppLogin, source.Namespace, system.OAuthAppLoginPrefix+source.Name); apierror.IsNotFound(err) {
		source.Status.Auth = types.OAuthAppLoginAuthStatus{
			Required: &required,
		}
		return nil
	} else if err != nil {
		return err
	}

	source.Status.Auth = oauthAppLogin.Status.OAuthAppLoginAuthStatus
	return req.Client.Status().Update(req.Ctx, source)
}

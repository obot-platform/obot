package knowledgefile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/nah/pkg/typed"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/invoke"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type UnsupportedError struct {
	UnsupportedFiletype string `json:"unsupportedFiletype"`
}

func (u *UnsupportedError) Error() string {
	return fmt.Sprintf("unsupported filetype: %s", u.UnsupportedFiletype)
}

type Handler struct {
	invoker   *invoke.Invoker
	gptScript *gptscript.GPTScript
	limit     int
}

func New(invoker *invoke.Invoker, gptScript *gptscript.GPTScript, limit int) *Handler {
	return &Handler{
		invoker:   invoker,
		gptScript: gptScript,
		limit:     limit,
	}
}

func shouldReIngest(file *v1.KnowledgeFile) bool {
	return file.Spec.IngestGeneration > file.Status.IngestGeneration ||
		file.Spec.UpdatedAt != file.Status.UpdatedAt ||
		file.Spec.Checksum != file.Status.Checksum ||
		file.Spec.URL != file.Status.URL ||
		(file.Status.State == types.KnowledgeFileStateError && file.Status.RetryCount < 3)
}

func cleanInput(filename string) string {
	return strings.TrimSuffix(path.Join(".conversion", filename), ".md") + ".md" // migration - before, the cleaning step accepted md and outputted md, now it's html -> md
}

func OutputFile(filename string) string {
	return path.Join(".conversion", filename+".json")
}

func getThread(ctx context.Context, c kclient.Client, ks *v1.KnowledgeSet, source *v1.KnowledgeSource) (*v1.Thread, error) {
	var thread v1.Thread
	if source != nil && source.Status.ThreadName != "" {
		return &thread, c.Get(ctx, router.Key(ks.Namespace, source.Status.ThreadName), &thread)
	}
	return &thread, c.Get(ctx, router.Key(ks.Namespace, ks.Status.ThreadName), &thread)
}

func (h *Handler) IngestFile(req router.Request, _ router.Response) error {
	file := req.Object.(*v1.KnowledgeFile)

	var source v1.KnowledgeSource
	if file.Spec.KnowledgeSourceName != "" {
		if err := req.Client.Get(req.Ctx, router.Key(file.Namespace, file.Spec.KnowledgeSourceName), &source); err != nil {
			// NotFound is fine, Cleanup handler will handle things and end up deleting this file
			return kclient.IgnoreNotFound(err)
		}
	}

	var ks v1.KnowledgeSet
	if err := req.Client.Get(req.Ctx, router.Key(file.Namespace, file.Spec.KnowledgeSetName), &ks); err != nil {
		// NotFound is fine, Cleanup handler will handle things and end up deleting this file
		return kclient.IgnoreNotFound(err)
	}

	if ks.Status.TextEmbeddingModel == "" {
		// Wait for the embedding model to be set
		return nil
	}

	thread, err := getThread(req.Ctx, req.Client, &ks, &source)
	if err != nil {
		return kclient.IgnoreNotFound(err)
	}

	if file.Status.State == "" {
		// We don't need to save it right now, has no real impact since the API turns "" into pending
		file.Status.State = types.KnowledgeFileStatePending
	}

	if file.Status.State == types.KnowledgeFileStateIngesting {
		// Resuming from failed system restart, go back to pending
		file.Status.State = types.KnowledgeFileStatePending
		if err := req.Client.Status().Update(req.Ctx, file); err != nil {
			return err
		}
	}

	// Check approval
	if file.Spec.Approved == nil {
		switch {
		case source.Spec.Manifest.AutoApprove != nil && *source.Spec.Manifest.AutoApprove:
			file.Spec.Approved = typed.Pointer(true)
		case isFileMatchPrefixPattern(file.Spec.FileName, source.Spec.Manifest.FilePathPrefixExclude):
			file.Spec.Approved = typed.Pointer(false)
		case isFileMatchPrefixPattern(file.Spec.FileName, source.Spec.Manifest.FilePathPrefixInclude):
			file.Spec.Approved = typed.Pointer(true)
		}

		if file.Spec.Approved != nil {
			if err := req.Client.Update(req.Ctx, file); err != nil {
				return err
			}
		}
	}

	if file.Status.State.IsTerminal() && !shouldReIngest(file) {
		return nil
	}

	// We should be pending at this point, if not update to that state
	if file.Status.State != types.KnowledgeFileStatePending {
		file.Status.State = types.KnowledgeFileStatePending
		if err := req.Client.Status().Update(req.Ctx, file); err != nil {
			return err
		}
	}

	if file.Spec.Approved == nil || !*file.Spec.Approved {
		// Not approved, wait for user action
		return nil
	}

	// If files have been approved, check whether the current knowledge set's approved files has exceeded limit
	var files v1.KnowledgeFileList
	if err := req.Client.List(req.Ctx, &files, kclient.InNamespace(ks.Namespace), kclient.MatchingFields{
		"spec.knowledgeSetName": ks.Name,
	}); err != nil {
		return err
	}
	ingestedFilesCount := 0
	for _, f := range files.Items {
		if f.Status.State == types.KnowledgeFileStateIngested {
			ingestedFilesCount++
		}
	}
	if ingestedFilesCount >= h.limit {
		file.Status.State = types.KnowledgeFileStateError
		file.Status.Error = "You have reached the maximum of files you can ingest"
		file.Status.URL = file.Spec.URL
		file.Status.UpdatedAt = file.Spec.UpdatedAt
		file.Status.Checksum = file.Spec.Checksum
		file.Status.IngestGeneration = file.Spec.IngestGeneration
		file.Status.RetryCount = 3
		return req.Client.Status().Update(req.Ctx, file)
	}

	if err := h.ingest(req.Ctx, req.Client, file, &ks, &source, thread); err != nil {
		var unsupportedErr *UnsupportedError
		if errors.As(err, &unsupportedErr) {
			file.Status.State = types.KnowledgeFileStateUnsupported
		} else {
			file.Status.State = types.KnowledgeFileStateError
		}
		file.Status.Error = err.Error()
		file.Status.RetryCount++
	} else {
		file.Status.State = types.KnowledgeFileStateIngested
		file.Status.Error = ""
		file.Status.RetryCount = 0
	}

	file.Status.LastIngestionEndTime = metav1.Now()
	file.Status.URL = file.Spec.URL
	file.Status.UpdatedAt = file.Spec.UpdatedAt
	file.Status.Checksum = file.Spec.Checksum
	file.Status.IngestGeneration = file.Spec.IngestGeneration
	return req.Client.Status().Update(req.Ctx, file)
}

func (h *Handler) ingest(ctx context.Context, client kclient.Client, file *v1.KnowledgeFile, ks *v1.KnowledgeSet, source *v1.KnowledgeSource, thread *v1.Thread) error {
	file.Status.State = types.KnowledgeFileStateIngesting
	file.Status.Error = ""
	file.Status.LastIngestionStartTime = metav1.Now()
	file.Status.LastIngestionEndTime = metav1.Time{}
	file.Status.RunNames = nil
	if err := client.Status().Update(ctx, file); err != nil {
		return err
	}

	inputName := file.Spec.FileName

	// Clean website content (remove headers, footers, etc.)
	if source.Spec.Manifest.GetType() == types.KnowledgeSourceTypeWebsite && strings.HasSuffix(inputName, ".html") {
		mdOutput := cleanInput(file.Spec.FileName) + ".md"
		task, err := h.invoker.SystemTask(ctx, thread, system.WebsiteCleanTool, map[string]any{
			"input":  inputName,
			"output": mdOutput,
		})
		if err != nil {
			return err
		}
		defer task.Close()

		file.Status.RunNames = append(file.Status.RunNames, task.Run.Name)
		if err := client.Status().Update(ctx, file); err != nil {
			return err
		}

		_, err = task.Result(ctx)
		if err != nil {
			return fmt.Errorf("failed to clean website content: %v", err)
		}

		inputName = mdOutput
	}

	loadTask, err := h.invoker.SystemTask(ctx, thread, system.KnowledgeLoadTool, map[string]any{
		"input":  inputName,
		"output": OutputFile(file.Spec.FileName),
	}, invoke.SystemTaskOptions{
		Env: []string{"OPENAI_MODEL=" + string(types.DefaultModelAliasTypeVision)},
	})
	if err != nil {
		return err
	}
	defer loadTask.Close()

	file.Status.RunNames = append(file.Status.RunNames, loadTask.Run.Name)
	if err := client.Status().Update(ctx, file); err != nil {
		return err
	}

	loadResult, err := loadTask.Result(ctx)
	if err != nil {
		return err
	}
	var unsupportedErr UnsupportedError
	if json.Unmarshal([]byte(loadResult.Output), &unsupportedErr) == nil && unsupportedErr.UnsupportedFiletype != "" {
		return &unsupportedErr
	}

	ingestTask, err := h.invoker.SystemTask(ctx, thread, system.KnowledgeIngestTool, map[string]any{
		"input":   OutputFile(file.Spec.FileName),
		"dataset": ks.Namespace + "/" + ks.Name,
		"metadata_json": map[string]string{
			"url":               file.Spec.URL,
			"workspaceID":       thread.Status.WorkspaceID,
			"workspaceFileName": OutputFile(file.Spec.FileName),
		},
	}, invoke.SystemTaskOptions{
		Env:     []string{"OPENAI_EMBEDDING_MODEL=" + ks.Status.TextEmbeddingModel},
		Timeout: 1 * time.Hour,
	})
	if err != nil {
		return fmt.Errorf("failed to invoke ingestion task, error: %w", err)
	}
	defer ingestTask.Close()

	file.Status.RunNames = append(file.Status.RunNames, ingestTask.Run.Name)
	if err := client.Status().Update(ctx, file); err != nil {
		return err
	}

	_, err = ingestTask.Result(ctx)
	if err != nil {
		return fmt.Errorf("failed to ingest file: %v", err)
	}

	return nil
}

func (h *Handler) getWorkspaceID(ctx context.Context, c kclient.Client, ks *v1.KnowledgeSet, source *v1.KnowledgeSource) (string, error) {
	var workspace v1.Workspace

	if source != nil && source.Status.WorkspaceName != "" {
		if err := c.Get(ctx, router.Key(ks.Namespace, source.Status.WorkspaceName), &workspace); err != nil {
			return "", err
		}
		return workspace.Status.WorkspaceID, nil
	}

	if err := c.Get(ctx, router.Key(ks.Namespace, ks.Status.WorkspaceName), &workspace); err != nil {
		return "", err
	}

	return workspace.Status.WorkspaceID, nil
}

func (h *Handler) Unapproved(req router.Request, _ router.Response) error {
	file := req.Object.(*v1.KnowledgeFile)

	// Basically if it's not approved and not pending
	if !(file.Spec.Approved != nil && !*file.Spec.Approved &&
		file.Status.State != types.KnowledgeFileStatePending) {
		return nil
	}

	var ks v1.KnowledgeSet
	if err := req.Client.Get(req.Ctx, router.Key(file.Namespace, file.Spec.KnowledgeSetName), &ks); err != nil {
		return kclient.IgnoreNotFound(err)
	}

	var source *v1.KnowledgeSource
	if file.Spec.KnowledgeSourceName != "" {
		source = &v1.KnowledgeSource{}
		if err := req.Client.Get(req.Ctx, router.Key(file.Namespace, file.Spec.KnowledgeSourceName), source); kclient.IgnoreNotFound(err) != nil {
			return err
		}
	}

	thread, err := getThread(req.Ctx, req.Client, &ks, source)
	if err != nil {
		return kclient.IgnoreNotFound(err)
	}

	task, err := h.invoker.SystemTask(req.Ctx, thread, system.KnowledgeDeleteFileTool, map[string]any{
		"file":    OutputFile(file.Spec.FileName),
		"dataset": ks.Namespace + "/" + ks.Name,
	})
	if err != nil {
		return err
	}
	defer task.Close()

	_, err = task.Result(req.Ctx)
	if err != nil {
		if file.Status.State != types.KnowledgeFileStateError {
			file.Status.State = types.KnowledgeFileStateError
			file.Status.Error = err.Error()
			return req.Client.Status().Update(req.Ctx, file)
		}
		// purposely ignore error, as the state is recorded
		return nil
	}

	file.Status.State = types.KnowledgeFileStatePending
	file.Status.RunNames = []string{task.Run.Name}
	file.Status.Error = ""
	return req.Client.Status().Update(req.Ctx, file)
}

func (h *Handler) Cleanup(req router.Request, _ router.Response) error {
	file := req.Object.(*v1.KnowledgeFile)

	var ks v1.KnowledgeSet
	if err := req.Client.Get(req.Ctx, router.Key(file.Namespace, file.Spec.KnowledgeSetName), &ks); err != nil || !ks.DeletionTimestamp.IsZero() {
		// The workspace will be deleted and the knowledge set removed from knowledge with the knowledge set controller.
		return kclient.IgnoreNotFound(err)
	}

	var (
		source              *v1.KnowledgeSource
		removeFromWorkspace = true
	)
	if file.Spec.KnowledgeSourceName != "" {
		source = &v1.KnowledgeSource{}
		if err := req.Client.Get(req.Ctx, router.Key(file.Namespace, file.Spec.KnowledgeSourceName), source); apierrors.IsNotFound(err) || !source.DeletionTimestamp.IsZero() {
			// The workspace will be deleted when the knowledge source is removed.
			removeFromWorkspace = false
		} else if err != nil {
			return err
		}
	}

	if removeFromWorkspace {
		workspaceID, err := h.getWorkspaceID(req.Ctx, req.Client, &ks, source)
		if err != nil {
			return kclient.IgnoreNotFound(err)
		}

		if err = h.gptScript.DeleteFileInWorkspace(req.Ctx, file.Spec.FileName, gptscript.DeleteFileInWorkspaceOptions{
			WorkspaceID: workspaceID,
		}); err != nil {
			return err
		}

		if err = h.gptScript.DeleteFileInWorkspace(req.Ctx, cleanInput(file.Spec.FileName), gptscript.DeleteFileInWorkspaceOptions{
			WorkspaceID: workspaceID,
		}); err != nil {
			return err
		}

		if err = h.gptScript.DeleteFileInWorkspace(req.Ctx, OutputFile(file.Spec.FileName), gptscript.DeleteFileInWorkspaceOptions{
			WorkspaceID: workspaceID,
		}); err != nil {
			return err
		}
	}

	thread, err := getThread(req.Ctx, req.Client, &ks, source)
	if err != nil {
		return kclient.IgnoreNotFound(err)
	}

	task, err := h.invoker.SystemTask(req.Ctx, thread, system.KnowledgeDeleteFileTool, map[string]any{
		"file":    OutputFile(file.Spec.FileName),
		"dataset": ks.Namespace + "/" + ks.Name,
	})
	if err != nil {
		return err
	}
	defer task.Close()

	_, err = task.Result(req.Ctx)
	if err != nil {
		return fmt.Errorf("failed to delete knowledge file: %w", err)
	}
	return nil
}

func isFileMatchPrefixPattern(filePath string, patterns []string) bool {
	for _, pattern := range patterns {
		if strings.HasPrefix(strings.TrimPrefix(filePath, "/"), pattern) {
			return true
		}
	}

	return false
}

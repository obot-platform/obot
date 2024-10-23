package knowledge

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/acorn-io/baaah/pkg/router"
	"github.com/acorn-io/baaah/pkg/uncached"
	"github.com/gptscript-ai/go-gptscript"
	"github.com/oauth2-proxy/oauth2-proxy/v7/pkg/logger"
	"github.com/otto8-ai/otto8/apiclient/types"
	"github.com/otto8-ai/otto8/pkg/events"
	"github.com/otto8-ai/otto8/pkg/knowledge"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.gptscript.ai/v1"
	"github.com/otto8-ai/otto8/pkg/workspace"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	gptscript *gptscript.GPTScript
	ingester  *knowledge.Ingester
	events    *events.Emitter
}

func New(gClient *gptscript.GPTScript, ingester *knowledge.Ingester, events *events.Emitter) *Handler {
	return &Handler{
		gptscript: gClient,
		ingester:  ingester,
		events:    events,
	}
}

func (a *Handler) DeleteKnowledge(req router.Request, _ router.Response) error {
	ks := req.Object.(*v1.KnowledgeSet)

	run, err := a.ingester.DeleteKnowledge(req.Ctx, ks.Namespace, ks.Name)
	if err != nil {
		return err
	}

	runCurrent, resp, err := a.events.Watch(req.Ctx, ks.GetNamespace(), events.WatchOptions{
		Run: run.Run,
	})
	if err != nil {
		return err
	}

	for range resp {
	}

	if err := req.Client.Get(req.Ctx, router.Key(runCurrent.Namespace, runCurrent.Name), runCurrent); err != nil {
		return err
	}

	if runCurrent.Status.State != gptscript.Finished {
		return fmt.Errorf("knowledge deletion run did not finish: %s", runCurrent.Status.State)
	}

	return nil
}

func (a *Handler) IngestKnowledge(req router.Request, resp router.Response) error {
	ws := req.Object.(*v1.Workspace)
	if !ws.Spec.IsKnowledge || ws.Spec.KnowledgeSetName == "" {
		return nil
	}

	// The status handler will clean this up
	if ws.Status.IngestionRunName != "" {
		return nil
	}

	if !ws.Status.IngestionLastRunTime.IsZero() && ws.Status.IngestionLastRunTime.Add(30*time.Second).After(time.Now()) {
		resp.RetryAfter(10 * time.Second)
		return nil
	}

	var files v1.KnowledgeFileList
	if err := req.Client.List(req.Ctx, &files, kclient.InNamespace(ws.Namespace), kclient.MatchingFields{
		"spec.workspaceName": ws.Name,
	}); err != nil {
		return err
	}

	var approvedFiles v1.KnowledgeFileList
	for _, file := range files.Items {
		if file.Spec.Approved != nil && *file.Spec.Approved {
			approvedFiles.Items = append(approvedFiles.Items, file)
		}
	}

	if len(approvedFiles.Items) == 0 {
		ws.Status.IngestionRunHash = ""
		return nil
	}

	// Sleep for 5 seconds before invoking to fetch the files. In case when files are approved at the same time, the first invoke will
	// have partial file approve list. It will eventually have all files but the first ingestion will be incompleted. Sleeping for 5 seconds
	// so that we can make sure we wait for the approved file that happens at the same time
	time.Sleep(5 * time.Second)
	if err := req.Client.List(req.Ctx, uncached.List(&files), kclient.InNamespace(ws.Namespace), kclient.MatchingFields{
		"spec.workspaceName": ws.Name,
	}); err != nil {
		return err
	}

	sort.Slice(files.Items, func(i, j int) bool {
		return files.Items[i].UID < files.Items[j].UID
	})

	digest := sha256.New()

	for _, file := range files.Items {
		if file.Spec.Approved != nil && *file.Spec.Approved {
			digest.Write([]byte(file.Name))
			digest.Write([]byte{0})
			digest.Write([]byte(file.Status.FileDetails.UpdatedAt))
			digest.Write([]byte{0})
		}
	}
	var syncNeeded bool

	hash := fmt.Sprintf("%x", digest.Sum(nil))
	if hash != ws.Status.IngestionRunHash {
		// Hash changed, always sync
		syncNeeded = true
		ws.Status.LastNotFinished = nil
		ws.Status.RetryCount = 0
	} else if len(ws.Status.NotFinished) > 0 {
		if maps.Equal(ws.Status.NotFinished, ws.Status.LastNotFinished) {
			// No progress made
			ws.Status.RetryCount++
			if ws.Status.RetryCount < 3 {
				// Retry again if we haven't retried 3 times
				ws.Status.LastNotFinished = ws.Status.NotFinished
				syncNeeded = true
			}
		} else {
			// Progress made, retry, reset retry count
			ws.Status.LastNotFinished = ws.Status.NotFinished
			ws.Status.RetryCount = 0
			syncNeeded = true
		}
	}

	if syncNeeded {
		var ignoreFileContent string
		for _, file := range files.Items {
			if file.Spec.Approved == nil || !*file.Spec.Approved {
				if file.Status.FileDetails.FilePath != "" {
					rel, err := filepath.Rel(workspace.GetDir(ws.Status.WorkspaceID), file.Status.FileDetails.FilePath)
					if err != nil {
						return fmt.Errorf("failed to get relative path for file: %w", err)
					}
					ignoreFileContent += fmt.Sprintf("%s\n", rel)
				} else {
					ignoreFileContent += fmt.Sprintf("%s\n", file.Spec.FileName)
				}
			}
		}

		if ignoreFileContent != "" {
			err := a.gptscript.WriteFileInWorkspace(req.Ctx, ".knowignore", []byte(ignoreFileContent), gptscript.WriteFileInWorkspaceOptions{
				WorkspaceID: ws.Status.WorkspaceID,
			})
			if err != nil {
				return fmt.Errorf("failed to create knowledge metadata file: %w", err)
			}
		} else {
			if err := a.gptscript.DeleteFileInWorkspace(req.Ctx, ".knowignore", gptscript.DeleteFileInWorkspaceOptions{
				WorkspaceID: ws.Status.WorkspaceID,
			}); err != nil {
				return fmt.Errorf("failed to delete ignore file: %w", err)
			}
		}

		run, err := a.ingester.IngestKnowledge(req.Ctx, ws.GetNamespace(), ws.Spec.KnowledgeSetName, ws.Status.WorkspaceID)
		if err != nil {
			return err
		}

		// Reset knowledge set error before ingestion
		var knowledgeSet v1.KnowledgeSet
		if err := req.Get(&knowledgeSet, ws.Namespace, ws.Spec.KnowledgeSetName); err != nil {
			return err
		}
		if knowledgeSet.Status.IngestionError != "" {
			knowledgeSet.Status.IngestionError = ""
			if err := req.Client.Status().Update(req.Ctx, &knowledgeSet); err != nil {
				return err
			}
		}

		ws.Status.IngestionRunHash = hash
		ws.Status.IngestionRunName = run.Run.Name
		ws.Status.IngestionGeneration++
		return req.Client.Status().Update(req.Ctx, ws)
	}

	return nil
}

func toStream(events <-chan types.Progress) io.ReadCloser {
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		for event := range events {
			_, err := pw.Write([]byte(event.Content))
			if err != nil {
				logger.Errorf("failed to write to pipe: %s", err)
				// Drain
				for range events {
				}
				return
			}
		}
	}()
	return pr
}

func (a *Handler) UpdateFileStatus(req router.Request, _ router.Response) error {
	ws := req.Object.(*v1.Workspace)

	if ws.Status.IngestionRunName == "" {
		return nil
	}

	var run v1.Run
	if err := req.Get(&run, ws.Namespace, ws.Status.IngestionRunName); apierrors.IsNotFound(err) {
		if err := req.Get(uncached.Get(&run), ws.Namespace, ws.Status.IngestionRunName); apierrors.IsNotFound(err) {
			// Orphaned? User deleted the run? Solar flare?
			ws.Status.IngestionRunName = ""
		}
		return nil
	} else if err != nil {
		return err
	}

	_, progress, err := a.events.Watch(req.Ctx, ws.Namespace, events.WatchOptions{
		Run: &run,
	})
	if err != nil {
		return err
	}

	NotFinished, err := compileFileStatuses(req.Ctx, req.Client, ws, progress)
	if err != nil {
		return err
	}

	// Fetch ingestion error from run.status in go routine so that we don't block
	runName := ws.Status.IngestionRunName
	go func() {
		for {
			stop, err := updateIngestionError(req, ws, runName)
			if err != nil {
				logger.Errorf("failed to update ingestion error: %s", err)
				break
			}

			if stop {
				break
			}
			time.Sleep(time.Second * 5)
		}
	}()

	ws.Status.IngestionRunName = ""
	ws.Status.NotFinished = NotFinished
	ws.Status.IngestionLastRunTime = metav1.Now()
	return nil
}

func updateIngestionError(req router.Request, ws *v1.Workspace, runName string) (bool, error) {
	var run v1.Run
	if err := req.Get(uncached.Get(&run), ws.Namespace, runName); err != nil && !apierrors.IsNotFound(err) {
		return false, err
	} else if err == nil {
		var knowledgeSet v1.KnowledgeSet
		if err := req.Get(&knowledgeSet, ws.Namespace, ws.Spec.KnowledgeSetName); err != nil {
			return false, err
		}
		if run.Status.Error != knowledgeSet.Status.IngestionError {
			knowledgeSet.Status.IngestionError = run.Status.Error
			if err := req.Client.Status().Update(req.Ctx, &knowledgeSet); err != nil {
				return false, err
			}
		}
	}

	if run.Status.State.IsTerminal() {
		return true, nil
	}

	return false, nil
}

func compileFileStatuses(ctx context.Context, client kclient.Client, ws *v1.Workspace, progress <-chan types.Progress) (map[string]string, error) {
	input := toStream(progress)
	defer input.Close()
	scanner := bufio.NewScanner(input)

	final := map[string]string{}

	var errs []error
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "{") {
			continue
		}

		var ingestionStatus types.IngestionStatus
		if err := json.Unmarshal([]byte(line), &ingestionStatus); err != nil {
			errs = append(errs, fmt.Errorf("failed to unmarshal event: %s", err))
			continue
		}

		if ingestionStatus.Filepath == "" || ingestionStatus.Status == "" {
			// Not a file status log.
			continue
		}

		var file v1.KnowledgeFile
		if err := client.Get(ctx, router.Key(ws.GetNamespace(), v1.ObjectNameFromAbsolutePath(ingestionStatus.Filepath)), &file); apierrors.IsNotFound(err) {
			errs = append(errs, fmt.Errorf("knowledge file not found: %s", ingestionStatus.Filepath))
			continue
		} else if err != nil {
			errs = append(errs, fmt.Errorf("failed to get knowledge file: %s", err))
		}
		final[file.Name] = ingestionStatus.Status

		if ingestionStatus.Status == "finished" {
			delete(final, file.Name)
		}

		if !equality.Semantic.DeepEqual(file.Status.IngestionStatus, ingestionStatus) {
			file.Status.IngestionStatus = ingestionStatus
			if err := client.Status().Update(ctx, &file); err != nil {
				errs = append(errs, fmt.Errorf("failed to update knowledge file: %s", err))
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return final, errors.Join(errs...)
}

func (a *Handler) CleanupFile(req router.Request, resp router.Response) error {
	kFile := req.Object.(*v1.KnowledgeFile)

	var ws v1.Workspace
	if err := req.Get(&ws, kFile.Namespace, kFile.Spec.WorkspaceName); apierrors.IsNotFound(err) {
		// If the workspace object is not found, then the workspaces will be deleted and nothing needs to happen here.
		return nil
	} else if err != nil {
		return err
	}

	var notFoundErr *gptscript.NotFoundInWorkspaceError
	if err := a.gptscript.DeleteFileInWorkspace(req.Ctx, kFile.Spec.FileName, gptscript.DeleteFileInWorkspaceOptions{WorkspaceID: ws.Status.WorkspaceID}); err != nil && !errors.As(err, &notFoundErr) {
		return err
	}

	var knowledgeSet v1.KnowledgeSet
	if err := req.Get(uncached.Get(&knowledgeSet), kFile.Namespace, ws.Spec.KnowledgeSetName); apierrors.IsNotFound(err) {
		// if knowledge set has been deleted already, then we don't need to run delete file
		return nil
	}

	if _, err := a.ingester.DeleteKnowledgeFiles(req.Ctx, kFile.Namespace, filepath.Join(workspace.GetDir(ws.Status.WorkspaceID), kFile.Spec.FileName), ws.Spec.KnowledgeSetName); err != nil {
		return err
	}

	return nil
}

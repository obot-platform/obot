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
	wclient "github.com/otto8-ai/workspace-provider/pkg/client"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	workspaceClient   *wclient.Client
	ingester          *knowledge.Ingester
	events            *events.Emitter
	workspaceProvider string
}

func New(wc *wclient.Client, ingester *knowledge.Ingester, wp string, events *events.Emitter) *Handler {
	return &Handler{
		workspaceClient:   wc,
		ingester:          ingester,
		workspaceProvider: wp,
		events:            events,
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

func (a *Handler) isIngestionBlocked(ctx context.Context, c kclient.Client, ws *v1.Workspace) (bool, error) {
	var ks v1.KnowledgeSet
	if err := c.Get(ctx, router.Key(ws.Namespace, ws.Spec.KnowledgeSetName), &ks); err != nil {
		return false, err
	}

	var rks v1.RemoteKnowledgeSourceList
	if err := c.List(ctx, &rks, kclient.InNamespace(ws.Namespace), kclient.MatchingFields{
		"spec.knowledgeSetName": ks.Name,
	}); err != nil {
		return false, err
	}

	for _, rks := range rks.Items {
		if rks.Spec.Manifest.DisableIngestionAfterSync {
			return true, nil
		}
	}

	return false, nil
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

	if blocked, err := a.isIngestionBlocked(req.Ctx, req.Client, ws); blocked || err != nil {
		return err
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

	if len(files.Items) == 0 {
		return nil
	}

	sort.Slice(files.Items, func(i, j int) bool {
		return files.Items[i].UID < files.Items[j].UID
	})

	digest := sha256.New()

	for _, file := range files.Items {
		digest.Write([]byte(file.Name))
		digest.Write([]byte{0})
		digest.Write([]byte(file.Status.FileDetails.UpdatedAt))
		digest.Write([]byte{0})
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
		run, err := a.ingester.IngestKnowledge(req.Ctx, ws.GetNamespace(), ws.Spec.KnowledgeSetName, ws.Status.WorkspaceID)
		if err != nil {
			return err
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

	// All good
	ws.Status.IngestionRunName = ""
	ws.Status.NotFinished = NotFinished
	ws.Status.IngestionLastRunTime = metav1.Now()
	return nil
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

		if ingestionStatus.Status == "skipped" {
			// Don't record the rather useless skipped messages
			continue
		}

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

	if err := a.workspaceClient.DeleteFile(req.Ctx, ws.Status.WorkspaceID, kFile.Spec.FileName); err != nil {
		if errors.As(err, new(wclient.FileNotFoundError)) {
			// It is important to return nil here and not move forward because when bulk deleting files from a remote provider
			// (like OneDrive), the connector will remove the files from the local disk and the controller will remove the
			// KnowledgeFile objects. We don't want to kick off (possibly) numerous ingestion runs.
			return nil
		}
		return err
	}

	return nil
}

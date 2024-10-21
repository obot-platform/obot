package uploads

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/acorn-io/baaah/pkg/router"
	"github.com/acorn-io/baaah/pkg/uncached"
	"github.com/gptscript-ai/go-gptscript"
	"github.com/otto8-ai/otto8/apiclient/types"
	"github.com/otto8-ai/otto8/pkg/invoke"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.gptscript.ai/v1"
	"github.com/otto8-ai/otto8/pkg/system"
	"github.com/otto8-ai/otto8/pkg/workspace"
	"github.com/robfig/cron/v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type UploadHandler struct {
	invoker           *invoke.Invoker
	gptscript         *gptscript.GPTScript
	workspaceProvider string
}

func New(invoker *invoke.Invoker, gptscript *gptscript.GPTScript, workspaceProvider string) *UploadHandler {
	return &UploadHandler{
		invoker:           invoker,
		gptscript:         gptscript,
		workspaceProvider: workspaceProvider,
	}
}

// CreateThread will create a thread for the upload. This is needed so that we can supply the metadata file to the
// connector. This will check to ensure an ingestion is not currently running to avoid adding files to a directory that
// is currently being ingested.
func (u *UploadHandler) CreateThread(req router.Request, resp router.Response) error {
	remoteKnowledgeSource := req.Object.(*v1.RemoteKnowledgeSource)
	if remoteKnowledgeSource.Status.ThreadName != "" {
		return nil
	}

	reSyncNeeded := remoteKnowledgeSource.Status.LastReSyncStarted.IsZero()
	// If no resync is needed and the schedule is set, check to see if it is time to run again.
	if !reSyncNeeded && remoteKnowledgeSource.Spec.Manifest.SyncSchedule != "" {
		schedule, err := cron.ParseStandard(remoteKnowledgeSource.Spec.Manifest.SyncSchedule)
		if err != nil {
			return err
		}

		timeUntilNext := time.Until(schedule.Next(remoteKnowledgeSource.Status.LastReSyncStarted.Time))
		reSyncNeeded = timeUntilNext <= 0
		if !reSyncNeeded {
			resp.RetryAfter(timeUntilNext)
		}
	}

	// If no resync is needed, check to see if there are any resync requests.
	if !reSyncNeeded {
		var reSyncRequests v1.SyncUploadRequestList
		if err := req.List(&reSyncRequests, &client.ListOptions{
			Namespace: remoteKnowledgeSource.Namespace,
			FieldSelector: fields.SelectorFromSet(map[string]string{
				"spec.remoteKnowledgeSourceName": remoteKnowledgeSource.Name,
			}),
		}); err != nil {
			return err
		}

		for _, reSyncRequest := range reSyncRequests.Items {
			if reSyncRequest.CreationTimestamp.After(remoteKnowledgeSource.Status.LastReSyncStarted.Time) {
				reSyncNeeded = true
				break
			}
		}
	}

	if !reSyncNeeded {
		return nil
	}

	id, err := u.gptscript.CreateWorkspace(req.Ctx, u.workspaceProvider)
	if err != nil {
		return fmt.Errorf("failed to create upload workspace: %w", err)
	}

	thread := &v1.Thread{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.ThreadPrefix,
			Namespace:    req.Namespace,
		},
		Spec: v1.ThreadSpec{
			RemoteKnowledgeSourceName: remoteKnowledgeSource.Name,
			WorkspaceID:               id,
		},
	}

	if err = req.Client.Create(req.Ctx, thread); err != nil {
		_ = u.gptscript.DeleteWorkspace(req.Ctx, id)
		return err
	}

	remoteKnowledgeSource.Status.ThreadName = thread.Name
	return req.Client.Status().Update(req.Ctx, remoteKnowledgeSource)
}

// RunUpload will run the tool for getting the files. It will only run if the thread has been created and set on the status.
func (u *UploadHandler) RunUpload(req router.Request, _ router.Response) error {
	remoteKnowledgeSource := req.Object.(*v1.RemoteKnowledgeSource)

	if remoteKnowledgeSource.Status.ThreadName == "" || remoteKnowledgeSource.Status.RunName != "" {
		// If the thread hasn't been set, or there is already a run in progress, don't do anything.
		return nil
	}

	var thread v1.Thread
	if err := req.Get(&thread, remoteKnowledgeSource.Namespace, remoteKnowledgeSource.Status.ThreadName); apierrors.IsNotFound(err) {
		// Might not be in the cache yet.
		return nil
	} else if err != nil {
		return err
	}

	files, err := knowledgeFilesForUploadName(req, remoteKnowledgeSource.Namespace, remoteKnowledgeSource.Name)
	if err != nil {
		return err
	}

	ws, err := getWorkspace(req, remoteKnowledgeSource)
	if err != nil {
		return fmt.Errorf("failed to get parent status: %w", err)
	}

	// This gets set as the "output" field in the metadata file. It should be the same as what came out of the last run, if any.
	b, err := json.Marshal(map[string]any{
		"input":     remoteKnowledgeSource.Spec.Manifest.RemoteKnowledgeSourceInput,
		"outputDir": filepath.Join(workspace.GetDir(ws.Status.WorkspaceID), remoteKnowledgeSource.Name),
		"output": map[string]any{
			"files":  compileKnowledgeFilesForConnector(files),
			"status": remoteKnowledgeSource.Status.Status,
			"error":  remoteKnowledgeSource.Status.Error,
			"state":  remoteKnowledgeSource.Status.State,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	if err = u.gptscript.WriteFileInWorkspace(req.Ctx, thread.Spec.WorkspaceID, ".metadata.json", b); err != nil {
		return fmt.Errorf("failed to create metadata file: %w", err)
	}

	r, err := u.invoker.SystemActionWithThread(req.Ctx, &thread, string(remoteKnowledgeSource.Spec.Manifest.SourceType)+"-data-source", "")
	if err != nil {
		return err
	}

	remoteKnowledgeSource.Status.RunName = r.Run.Name
	remoteKnowledgeSource.Status.LastReSyncStarted = metav1.Now()
	// Immediate persist to avoid infinite loops of creating runs.
	return req.Client.Status().Update(req.Ctx, remoteKnowledgeSource)
}

// HandleUploadRun will read the output metadata from the connector tool once the run is finished. It will populate the
// FileDetails for each file downloaded, and create or update the knowledge metadata file in the parent's knowledge
// workspace. It will only process if the run has been created and set on the status.
func (u *UploadHandler) HandleUploadRun(req router.Request, resp router.Response) error {
	remoteKnowledgeSource := req.Object.(*v1.RemoteKnowledgeSource)

	if remoteKnowledgeSource.Status.RunName == "" {
		return nil
	}

	var run v1.Run
	if err := req.Get(&run, remoteKnowledgeSource.Namespace, remoteKnowledgeSource.Status.RunName); err != nil {
		return err
	}

	defer func() {
		if !run.Status.State.IsTerminal() {
			resp.RetryAfter(5 * time.Second)
		}
	}()

	var thread v1.Thread
	if err := req.Get(&thread, remoteKnowledgeSource.Namespace, remoteKnowledgeSource.Status.ThreadName); err != nil {
		return err
	}

	var metadata struct {
		Output v1.RemoteConnectorStatus `json:"output,omitempty"`
	}

	file, err := u.gptscript.ReadFileInWorkspace(req.Ctx, thread.Spec.WorkspaceID, ".metadata.json")
	if err != nil {
		// Purposely ignore not found errors.
		if !strings.HasPrefix(err.Error(), "not found") {
			return err
		}
	} else {
		if err = json.Unmarshal(file, &metadata); err != nil {
			return err
		}

		remoteKnowledgeSource.Status.Status = metadata.Output.Status
		remoteKnowledgeSource.Status.Error = metadata.Output.Error
	}

	// If the run is in a terminal state, then we need to compile the file statuses and pass them off to knowledge.
	ws, err := getWorkspace(req, remoteKnowledgeSource)
	if err != nil {
		return err
	}

	knowledgeFileNamesFromOutput, err := compileKnowledgeFiles(req.Ctx, req.Client, remoteKnowledgeSource, metadata.Output.Files, ws)
	if err != nil {
		return err
	}

	if err := u.writeMetadataForKnowledge(req.Ctx, metadata.Output.Files, ws, remoteKnowledgeSource); err != nil {
		return err
	}

	remoteKnowledgeSource.Status.State = metadata.Output.State

	if run.Status.State.IsTerminal() {
		if err = deleteKnowledgeFilesNotIncluded(req.Ctx, req.Client, remoteKnowledgeSource.Namespace, remoteKnowledgeSource.Name, knowledgeFileNamesFromOutput); err != nil {
			return err
		}

		// Reset run name to indicate that the run is no longer running
		remoteKnowledgeSource.Status.ThreadName = ""
		remoteKnowledgeSource.Status.RunName = ""
		return req.Client.Status().Update(req.Ctx, remoteKnowledgeSource)
	}

	return nil
}

func (u *UploadHandler) writeMetadataForKnowledge(ctx context.Context, files map[string]types.FileDetails,
	ws *v1.Workspace, remoteKnowledgeSource *v1.RemoteKnowledgeSource) error {
	b, err := json.Marshal(map[string]any{
		"metadata": createFileMetadata(files, *ws),
	})
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Put the metadata file in the agent knowledge workspace
	return u.gptscript.WriteFileInWorkspace(ctx, ws.Status.WorkspaceID, filepath.Join(remoteKnowledgeSource.Name, ".knowledge.json"), b)
}

func (u *UploadHandler) Cleanup(req router.Request, resp router.Response) error {
	remoteKnowledgeSource := req.Object.(*v1.RemoteKnowledgeSource)

	ws, err := getWorkspace(req, remoteKnowledgeSource)
	if apierrors.IsNotFound(err) {
		// If the parent object is gone, then other handlers will ensure things are cleaned up.
		return nil
	} else if err != nil {
		return err
	}

	// Delete the directory in the workspace for this onedrive link.
	if err = u.gptscript.RemoveAllWithPrefix(req.Ctx, ws.Status.WorkspaceID, remoteKnowledgeSource.Name); err != nil {
		return fmt.Errorf("failed to delete directory %q in workspace %s: %w", remoteKnowledgeSource.Name, ws.Status.WorkspaceID, err)
	}

	return nil
}

func getWorkspace(req router.Request, remoteKnowledgeSource *v1.RemoteKnowledgeSource) (*v1.Workspace, error) {
	var ks v1.KnowledgeSet
	if err := req.Get(&ks, remoteKnowledgeSource.Namespace, remoteKnowledgeSource.Spec.KnowledgeSetName); err != nil {
		return nil, err
	}
	var ws v1.Workspace
	return &ws, req.Get(&ws, remoteKnowledgeSource.Namespace, ks.Status.WorkspaceName)
}

func knowledgeFilesForUploadName(req router.Request, namespace, name string) (v1.KnowledgeFileList, error) {
	var files v1.KnowledgeFileList
	return files, req.List(&files, &client.ListOptions{
		FieldSelector: fields.SelectorFromSet(map[string]string{"spec.remoteKnowledgeSourceName": name}),
		Namespace:     namespace,
	})
}

func createFileMetadata(files map[string]types.FileDetails, ws v1.Workspace) map[string]any {
	var (
		// fileMetadata is the metadata for the knowledge tool, translated from the connector output.
		fileMetadata = make(map[string]any, len(files))
		outputDir    = workspace.GetDir(ws.Status.WorkspaceID)
	)
	for _, v := range files {
		fileRelPath, err := filepath.Rel(outputDir, v.FilePath)
		if err != nil {
			fileRelPath = v.FilePath
		}

		fileMetadata[fileRelPath] = map[string]any{
			"source": v.URL,
		}
	}
	return fileMetadata
}

func compileKnowledgeFiles(ctx context.Context, c client.Client,
	remoteKnowledgeSource *v1.RemoteKnowledgeSource, files map[string]types.FileDetails,
	ws *v1.Workspace) (map[string]struct{}, error) {
	var (
		errs []error
		// fileMetadata is the metadata for the knowledge tool, translated from the connector output.
		outputDir                    = workspace.GetDir(ws.Status.WorkspaceID)
		knowledgeFileNamesFromOutput = make(map[string]struct{}, len(files))
	)

	for id, v := range files {
		fileRelPath, err := filepath.Rel(outputDir, v.FilePath)
		if err != nil {
			fileRelPath = v.FilePath
		}

		name := v1.ObjectNameFromAbsolutePath(v.FilePath)
		knowledgeFileNamesFromOutput[name] = struct{}{}
		newKnowledgeFile := &v1.KnowledgeFile{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: remoteKnowledgeSource.Namespace,
			},
			Spec: v1.KnowledgeFileSpec{
				WorkspaceName:             ws.Name,
				FileName:                  fileRelPath,
				RemoteKnowledgeSourceName: remoteKnowledgeSource.Name,
				RemoteKnowledgeSourceType: remoteKnowledgeSource.Spec.Manifest.SourceType,
			},
		}
		if remoteKnowledgeSource.Spec.Manifest.AutoApprove != nil && *remoteKnowledgeSource.Spec.Manifest.AutoApprove {
			newKnowledgeFile.Spec.Approved = &[]bool{true}[0]
		}
		if err := c.Create(ctx, newKnowledgeFile); err == nil || apierrors.IsAlreadyExists(err) {
			// If the file was created or already existed, ensure it has the latest details from the metadata.
			if err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
				if err := c.Get(ctx, router.Key(newKnowledgeFile.Namespace, newKnowledgeFile.Name), uncached.Get(newKnowledgeFile)); err != nil {
					return err
				}
				v.Ingested = newKnowledgeFile.Status.IngestionStatus.Status == "finished" || newKnowledgeFile.Status.IngestionStatus.Status == "skipped"
				if newKnowledgeFile.Status.UploadID == id && newKnowledgeFile.Status.FileDetails == v {
					// The file has the correct details, no need to update.
					return nil
				}

				newKnowledgeFile.Status.FileDetails = v
				newKnowledgeFile.Status.UploadID = id
				return c.Status().Update(ctx, newKnowledgeFile)
			}); err != nil {
				errs = append(errs, fmt.Errorf("failed to update knowledge file %q status: %w", newKnowledgeFile.Name, err))
			}
		} else if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("failed to create knowledge files: %w", errors.Join(errs...))
	}

	return knowledgeFileNamesFromOutput, nil
}

func deleteKnowledgeFilesNotIncluded(ctx context.Context, c client.Client, namespace, name string, filenames map[string]struct{}) error {
	var knowledgeFiles v1.KnowledgeFileList
	if err := c.List(ctx, uncached.List(&knowledgeFiles), &client.ListOptions{
		Namespace:     namespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{"spec.remoteKnowledgeSourceName": name}),
	}); err != nil {
		return fmt.Errorf("failed to list knowledge files: %w", err)
	}

	var errs []error
	for _, knowledgeFile := range knowledgeFiles.Items {
		if _, exists := filenames[knowledgeFile.Name]; !exists {
			if err := c.Delete(ctx, &knowledgeFile); err != nil {
				errs = append(errs, fmt.Errorf("failed to delete knowledge file %q: %w", knowledgeFile.Name, err))
			}
		}
	}

	return errors.Join(errs...)
}

func compileKnowledgeFilesForConnector(files v1.KnowledgeFileList) map[string]types.FileDetails {
	knowledgeFileStatuses := make(map[string]types.FileDetails, len(files.Items))
	for _, file := range files.Items {
		knowledgeFileStatuses[file.Status.UploadID] = file.Status.FileDetails
	}

	return knowledgeFileStatuses
}

package uploads

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/acorn-io/baaah/pkg/apply"
	"github.com/acorn-io/baaah/pkg/router"
	"github.com/acorn-io/baaah/pkg/uncached"
	"github.com/gptscript-ai/otto/apiclient/types"
	"github.com/gptscript-ai/otto/pkg/invoke"
	v1 "github.com/gptscript-ai/otto/pkg/storage/apis/otto.gptscript.ai/v1"
	"github.com/gptscript-ai/otto/pkg/system"
	"github.com/gptscript-ai/otto/pkg/workspace"
	"github.com/robfig/cron/v3"
	wclient "github.com/thedadams/workspace-provider/pkg/client"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type UploadHandler struct {
	invoker             *invoke.Invoker
	workspaceClient     *wclient.Client
	workspaceProvider   string
	onedriveTool        string
	notionTool          string
	websiteCrawlingTool string
}

func New(invoker *invoke.Invoker, wc *wclient.Client, workspaceProvider, onedriveTool, notionTool, websiteCrawlingTool string) *UploadHandler {
	return &UploadHandler{
		invoker:             invoker,
		workspaceClient:     wc,
		workspaceProvider:   workspaceProvider,
		onedriveTool:        onedriveTool,
		notionTool:          notionTool,
		websiteCrawlingTool: websiteCrawlingTool,
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

	ws, err := knowledgeWorkspaceFromParent(req, remoteKnowledgeSource)
	if apierrors.IsNotFound(err) {
		// A not found error indicates that things are getting cleaned up.
		// Ignore it because the cleanup handler will ensure this object is deleted.
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to get parent status: %w", err)
	}

	if ws.Status.IngestionRunName != "" {
		// Check to see if the ingestion run is still running.
		var run v1.Run
		if err = req.Get(&run, ws.Namespace, ws.Status.IngestionRunName); err != nil && !apierrors.IsNotFound(err) {
			return err
		} else if err == nil && !run.Status.State.IsTerminal() {
			// An ingestion is running. Don't download files while that is happening because it may corrupt things.
			return nil
		}
	}

	reSyncNeeded := remoteKnowledgeSource.Status.LastReSyncStarted.IsZero()
	// If no resync is needed and the schedule is set, check to see if it is time to run again.
	if !reSyncNeeded && remoteKnowledgeSource.Spec.SyncSchedule != "" {
		schedule, err := cron.ParseStandard(remoteKnowledgeSource.Spec.SyncSchedule)
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
		if err = req.List(&reSyncRequests, &client.ListOptions{
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

	id, err := u.workspaceClient.Create(req.Ctx, u.workspaceProvider)
	if err != nil {
		return fmt.Errorf("failed to create upload workspace: %w", err)
	}

	thread := &v1.Thread{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.ThreadPrefix,
			Namespace:    req.Namespace,
			Labels: map[string]string{
				v1.RemoteKnowledgeSourceLabel: remoteKnowledgeSource.Name,
			},
		},
		Spec: v1.ThreadSpec{
			AgentName:    remoteKnowledgeSource.Spec.AgentName,
			WorkflowName: remoteKnowledgeSource.Spec.WorkflowName,
			WorkspaceID:  id,
		},
	}
	if err = req.Client.Create(req.Ctx, thread); err != nil {
		_ = u.workspaceClient.Rm(req.Ctx, id)
		return err
	}

	remoteKnowledgeSource.Status.ThreadName = thread.Name

	return nil
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

	// This gets set as the "output" field in the metadata file. It should be the same as what came out of the last run, if any.
	output := map[string]any{
		"files":  compileKnowledgeFilesForConnector(files),
		"status": remoteKnowledgeSource.Status.Status,
		"error":  remoteKnowledgeSource.Status.Error,
		"state":  remoteKnowledgeSource.Status.State,
	}

	ws, err := knowledgeWorkspaceFromParent(req, remoteKnowledgeSource)
	if err != nil {
		return fmt.Errorf("failed to get parent status: %w", err)
	}

	b, err := json.Marshal(map[string]any{
		"input":     remoteKnowledgeSource.Spec.RemoteKnowledgeSourceInput,
		"outputDir": filepath.Join(workspace.GetDir(ws.Status.WorkspaceID), remoteKnowledgeSource.Name),
		"output":    output,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	writer, err := u.workspaceClient.WriteFile(req.Ctx, thread.Spec.WorkspaceID, ".metadata.json")
	if err != nil {
		return fmt.Errorf("failed to create metadata file: %w", err)
	}

	if _, err = writer.Write(b); err != nil {
		_ = writer.Close()
		return fmt.Errorf("failed to write metadata file: %w", err)
	}

	// The file has to be closed before the system action is started to ensure that the tool has the metadata file.
	if err = writer.Close(); err != nil {
		return fmt.Errorf("failed to close metadata file: %w", err)
	}

	r, err := u.invoker.SystemActionWithThread(req.Ctx, &thread, u.getTool(remoteKnowledgeSource), "{}")
	if err != nil {
		return err
	}

	go func() {
		// Don't care about the events here, but we need to pull them out
		r.Wait()
	}()

	remoteKnowledgeSource.Status.RunName = r.Run.Name
	remoteKnowledgeSource.Status.LastReSyncStarted = metav1.Now()
	return nil
}

func (u *UploadHandler) getTool(r *v1.RemoteKnowledgeSource) string {
	switch r.Spec.SourceType {
	case types.RemoteKnowledgeSourceTypeOneDrive:
		return u.onedriveTool
	case types.RemoteKnowledgeSourceTypeNotion:
		return u.notionTool
	case types.RemoteKnowledgeSourceTypeWebsite:
		return u.websiteCrawlingTool
	default:
		return ""
	}
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
	if err := req.Get(&run, remoteKnowledgeSource.Namespace, remoteKnowledgeSource.Status.RunName); apierrors.IsNotFound(err) {
		// Might not be in the cache yet.
		return nil
	} else if err != nil {
		return err
	}

	var thread v1.Thread
	if err := req.Get(&thread, remoteKnowledgeSource.Namespace, remoteKnowledgeSource.Status.ThreadName); err != nil {
		return err
	}

	file, err := u.workspaceClient.OpenFile(req.Ctx, thread.Spec.WorkspaceID, ".metadata.json")
	if err != nil {
		return err
	}
	defer file.Close()

	var metadata struct {
		Output v1.RemoteConnectorStatus `json:"output,omitempty"`
	}
	if err = json.NewDecoder(file).Decode(&metadata); err != nil {
		return err
	}

	remoteKnowledgeSource.Status.Status = metadata.Output.Status
	remoteKnowledgeSource.Status.Error = metadata.Output.Error

	if !run.Status.State.IsTerminal() {
		resp.RetryAfter(5 * time.Second)
		return nil
	}

	// If the run is in a terminal state, then we need to compile the file statuses and pass them off to knowledge.
	ws, err := knowledgeWorkspaceFromParent(req, remoteKnowledgeSource)
	if err != nil {
		return err
	}

	fileMetadata, knowledgeFileNamesFromOutput, err := compileKnowledgeFilesFromOneDriveConnector(req.Ctx, req.Client, remoteKnowledgeSource, metadata.Output.Files, ws)
	if err != nil {
		return err
	}

	if err = deleteKnowledgeFilesNotIncluded(req.Ctx, req.Client, remoteKnowledgeSource.Namespace, remoteKnowledgeSource.Name, knowledgeFileNamesFromOutput); err != nil {
		return err
	}

	// Put the metadata file in the agent knowledge workspace
	writer, err := u.workspaceClient.WriteFile(req.Ctx, ws.Status.WorkspaceID, filepath.Join(remoteKnowledgeSource.Name, ".knowledge.json"), wclient.WriteOptions{CreateDirs: true})
	if err != nil {
		return fmt.Errorf("failed to create knowledge metadata file: %w", err)
	}
	defer writer.Close()

	if err = json.NewEncoder(writer).Encode(map[string]any{"metadata": fileMetadata}); err != nil {
		return fmt.Errorf("failed to encode metadata file: %w", err)
	}

	remoteKnowledgeSource.Status.State = metadata.Output.State

	// Reset run name to indicate that the run is no longer running
	remoteKnowledgeSource.Status.ThreadName = ""
	remoteKnowledgeSource.Status.RunName = ""

	// Create object to re-ingest knowledge
	resp.Objects(
		&v1.IngestKnowledgeRequest{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: system.IngestRequestPrefix,
				Namespace:    req.Namespace,
				Annotations: map[string]string{
					// Don't prune because the cleanup handler will do that.
					apply.AnnotationPrune: "false",
				},
			},
			Spec: v1.IngestKnowledgeRequestSpec{
				WorkspaceName: ws.Name,
				HasKnowledge:  len(fileMetadata) > 0,
			},
		},
	)

	return nil
}

func (u *UploadHandler) Cleanup(req router.Request, resp router.Response) error {
	remoteKnowledgeSource := req.Object.(*v1.RemoteKnowledgeSource)

	// Delete the threads associated with the onedrive links. The runs will be cleaned up by their cleanup handler.
	var threads v1.ThreadList
	if err := req.List(&threads, &client.ListOptions{
		Namespace: remoteKnowledgeSource.Namespace,
		LabelSelector: labels.SelectorFromSet(map[string]string{
			v1.RemoteKnowledgeSourceLabel: remoteKnowledgeSource.Name,
		}),
	}); err != nil {
		return err
	}

	for _, thread := range threads.Items {
		if err := client.IgnoreNotFound(req.Delete(&thread)); err != nil {
			return fmt.Errorf("failed to delete thread %q: %w", thread.Name, err)
		}
	}

	ws, err := knowledgeWorkspaceFromParent(req, remoteKnowledgeSource)
	if apierrors.IsNotFound(err) {
		// If the parent object is gone, then other handlers will ensure things are cleaned up.
		return nil
	} else if err != nil {
		return err
	}

	// Delete the directory in the workspace for this onedrive link.
	if err = u.workspaceClient.RmDir(req.Ctx, ws.Status.WorkspaceID, remoteKnowledgeSource.Name); err != nil {
		return fmt.Errorf("failed to delete directory %q in workspace %s: %w", remoteKnowledgeSource.Name, ws.Status.WorkspaceID, err)
	}

	files, err := u.workspaceClient.Ls(req.Ctx, ws.Status.WorkspaceID)
	if err != nil {
		return fmt.Errorf("failed to list files in workspace %s: %w", ws.Status.WorkspaceID, err)
	}

	resp.Objects(
		&v1.IngestKnowledgeRequest{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: system.IngestRequestPrefix,
				Namespace:    req.Namespace,
				Annotations: map[string]string{
					// Don't prune because the cleanup handler will do that.
					apply.AnnotationPrune: "false",
				},
			},
			Spec: v1.IngestKnowledgeRequestSpec{
				WorkspaceName: ws.Name,
				HasKnowledge:  len(files) > 0,
			},
		},
	)

	return nil
}

func knowledgeWorkspaceFromParent(req router.Request, remoteKnowledgeSource *v1.RemoteKnowledgeSource) (*v1.Workspace, error) {
	if remoteKnowledgeSource.Spec.AgentName != "" {
		var agent v1.Agent
		if err := req.Get(&agent, remoteKnowledgeSource.Namespace, remoteKnowledgeSource.Spec.AgentName); err != nil {
			return nil, err
		}

		var ws v1.Workspace
		return &ws, req.Get(&ws, agent.Namespace, agent.Status.KnowledgeWorkspaceName)
	} else if remoteKnowledgeSource.Spec.WorkflowName != "" {
		var workflow v1.Workflow
		if err := req.Get(&workflow, remoteKnowledgeSource.Namespace, remoteKnowledgeSource.Spec.WorkflowName); err != nil {
			return nil, err
		}

		var ws v1.Workspace
		return &ws, req.Get(&ws, workflow.Namespace, workflow.Status.KnowledgeWorkspaceName)
	}

	return nil, fmt.Errorf("no parent object found for RemoteKnowledgeSource %q", remoteKnowledgeSource.Name)
}

func knowledgeFilesForUploadName(req router.Request, namespace, name string) (v1.KnowledgeFileList, error) {
	var files v1.KnowledgeFileList
	return files, req.List(&files, &client.ListOptions{
		FieldSelector: fields.SelectorFromSet(map[string]string{"spec.remoteKnowledgeSourceName": name}),
		Namespace:     namespace,
	})
}

func compileKnowledgeFilesFromOneDriveConnector(ctx context.Context, c client.Client, remoteKnowledgeSource *v1.RemoteKnowledgeSource, files map[string]types.FileDetails, ws *v1.Workspace) (map[string]any, map[string]struct{}, error) {
	var (
		errs []error
		// fileMetadata is the metadata for the knowledge tool, translated from the connector output.
		fileMetadata                 = make(map[string]any, len(files))
		outputDir                    = workspace.GetDir(ws.Status.WorkspaceID)
		knowledgeFileNamesFromOutput = make(map[string]struct{}, len(files))
	)
	for id, v := range files {
		fileRelPath, err := filepath.Rel(outputDir, v.FilePath)
		if err != nil {
			fileRelPath = v.FilePath
		}

		fileMetadata[fileRelPath] = map[string]any{
			"source": v.URL,
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
				RemoteKnowledgeSourceType: remoteKnowledgeSource.Spec.SourceType,
			},
		}
		if err := c.Create(ctx, newKnowledgeFile); err == nil || apierrors.IsAlreadyExists(err) {
			// If the file was created or already existed, ensure it has the latest details from the metadata.
			if err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
				if err := c.Get(ctx, router.Key(newKnowledgeFile.Namespace, newKnowledgeFile.Name), uncached.Get(newKnowledgeFile)); err != nil {
					return err
				}
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
		return nil, nil, fmt.Errorf("failed to create knowledge files: %w", errors.Join(errs...))
	}

	return fileMetadata, knowledgeFileNamesFromOutput, nil
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

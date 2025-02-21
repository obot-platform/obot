package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/selectors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type FilesHandler struct {
	gptScript *gptscript.GPTScript
}

func NewFilesHandler(gClient *gptscript.GPTScript) *FilesHandler {
	return &FilesHandler{
		gptScript: gClient,
	}
}

func (f *FilesHandler) Files(req api.Context) error {
	thread, err := getThreadForScope(req)
	if apierrors.IsNotFound(err) {
		return req.Write(types.FileList{Items: []types.File{}})
	} else if err != nil {
		return err
	}

	if thread.Status.WorkspaceID == "" {
		return req.Write(types.FileList{Items: []types.File{}})
	}

	return listFileFromWorkspace(req.Context(), req, f.gptScript, gptscript.ListFilesInWorkspaceOptions{
		WorkspaceID: thread.Status.WorkspaceID,
		Prefix:      "files/",
	})
}

func (f *FilesHandler) GetFile(req api.Context) error {
	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	if thread.Status.WorkspaceID == "" {
		return types.NewErrNotFound("no workspace found")
	}

	return getFileInWorkspace(req.Context(), req, f.gptScript, thread.Status.WorkspaceID, "files/")
}

func (f *FilesHandler) UploadFile(req api.Context) error {
	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	if thread.Status.WorkspaceID == "" {
		return types.NewErrNotFound("no workspace found")
	}

	_, err = uploadFileToWorkspace(req.Context(), req, f.gptScript, thread.Status.WorkspaceID, "files/")
	return err
}

func (f *FilesHandler) DeleteFile(req api.Context) error {
	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	if thread.Status.WorkspaceID == "" {
		return nil
	}

	return deleteFileFromWorkspaceID(req.Context(), req, f.gptScript, thread.Status.WorkspaceID, "files/")
}

func listFiles(ctx context.Context, req api.Context, gClient *gptscript.GPTScript, workspaceName string) error {
	var ws v1.Workspace
	if err := req.Get(&ws, workspaceName); err != nil {
		return err
	}

	return listFileFromWorkspace(ctx, req, gClient, gptscript.ListFilesInWorkspaceOptions{
		WorkspaceID: ws.Status.WorkspaceID,
		Prefix:      "files/",
	})
}

func listFileFromWorkspace(ctx context.Context, req api.Context, gClient *gptscript.GPTScript, opts gptscript.ListFilesInWorkspaceOptions) error {
	if opts.WorkspaceID == "" {
		return types.NewErrHTTP(http.StatusTooEarly, "workspace is not available yet")
	}
	files, err := gClient.ListFilesInWorkspace(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to list files in workspace %q: %w", opts.WorkspaceID, err)
	}

	return req.Write(types.FileList{Items: compileFileNames(files, opts)})
}

func getWorkspaceFromKnowledgeSet(req api.Context, knowledgeSetName string) (*v1.Workspace, error) {
	var knowledgeSet v1.KnowledgeSet
	if err := req.Get(&knowledgeSet, knowledgeSetName); err != nil {
		return nil, err
	}

	var ws v1.Workspace
	return &ws, req.Get(&ws, knowledgeSet.Status.WorkspaceName)
}

// getKnowledgeFile retrieves a knowledge file from the workspace associated with the knowledge set.
// It works for both thread and agent knowledge sets. If the knowledge set is not found in the thread, it will be looked up in the agent.
func getKnowledgeFile(req api.Context, gClient *gptscript.GPTScript, thread *v1.Thread, agent *v1.Agent, fileRef string) error {
	var err error

	// make sure that the selected knowledge set belongs either to the thread or to the agent
	var knowledgeSetNames []string
	if thread != nil {
		knowledgeSetNames = thread.Status.KnowledgeSetNames
		if agent == nil {
			agent, err = getAssistant(req, thread.Spec.AgentName)
			if err != nil {
				return err
			}
		}
	}

	if agent != nil {
		knowledgeSetNames = append(knowledgeSetNames, agent.Status.KnowledgeSetNames...)
	}

	return getKnowledgeFileFromAllowedSets(req, gClient, knowledgeSetNames, fileRef)
}

// getKnowledgeFileFromAllowedSets retrieves a knowledge file from the workspace associated with the knowledge set, if the knowledge set is in the list of allowed knowledge sets.
// The fileRef is expected to be in the URL-encoded format [<knowledgeSet.Namespace>/]<knowledgeSet.Name>::<filename>.
func getKnowledgeFileFromAllowedSets(req api.Context, gClient *gptscript.GPTScript, knowledgeSetNames []string, fileRef string) error {
	var knowledgeSetName string

	file, err := url.PathUnescape(fileRef)
	if err != nil {
		return types.NewErrBadRequest("invalid knowledgeFile reference")
	}

	parts := strings.Split(file, "::")
	if len(parts) != 2 {
		return types.NewErrBadRequest("invalid knowledgeFile path")
	}
	knowledgeSetName, file = parts[0], parts[1]

	if parts := strings.Split(knowledgeSetName, "/"); len(parts) > 1 {
		knowledgeSetName = parts[1] // may come in as <namespace>/<knowledgeset>, we don't care about the namespace right now
	}

	if !slices.Contains(knowledgeSetNames, knowledgeSetName) {
		return types.NewErrNotFound("knowledge set %q not accessible", knowledgeSetName)
	}

	ws, err := getWorkspaceFromKnowledgeSet(req, knowledgeSetName)
	if err != nil {
		return err
	}

	req.SetPathValue("file", file)
	return getFileInWorkspace(req.Context(), req, gClient, ws.Status.WorkspaceID, "") // knowledge files are stored in the root of the workspace (we have one workspace per knowledge set)
}

func listKnowledgeFiles(req api.Context, agentName, threadName, knowledgeSetName string, knowledgeSource *v1.KnowledgeSource) error {
	var (
		files               v1.KnowledgeFileList
		knowledgeSourceName string
	)
	if knowledgeSource != nil {
		knowledgeSourceName = knowledgeSource.Name
	}

	if err := req.List(&files, &client.ListOptions{
		FieldSelector: fields.SelectorFromSet(selectors.RemoveEmpty(map[string]string{
			"spec.knowledgeSetName":    knowledgeSetName,
			"spec.knowledgeSourceName": knowledgeSourceName,
		})),
	}); err != nil {
		return err
	}

	autoApprove := knowledgeSource == nil || (knowledgeSource.Spec.Manifest.AutoApprove != nil && *knowledgeSource.Spec.Manifest.AutoApprove)
	resp := make([]types.KnowledgeFile, 0, len(files.Items))
	for _, file := range files.Items {
		if knowledgeSourceName == "" && file.Spec.KnowledgeSourceName != "" {
			continue
		}
		if file.Spec.Approved == nil && autoApprove {
			file.Spec.Approved = &[]bool{true}[0]
		}
		resp = append(resp, convertKnowledgeFile(agentName, threadName, file))
	}

	return req.Write(types.KnowledgeFileList{Items: resp})
}

func uploadKnowledgeToWorkspace(req api.Context, gClient *gptscript.GPTScript, ws *v1.Workspace, agentName, threadName, knowledgeSetName string) error {
	filename := req.PathValue("file")

	size, err := uploadFileToWorkspace(req.Context(), req, gClient, ws.Status.WorkspaceID, "", api.BodyOptions{
		// 100MB
		MaxBytes: 100 * 1024 * 1024,
	})
	if err != nil {
		return err
	}

	file := v1.KnowledgeFile{
		ObjectMeta: metav1.ObjectMeta{
			Name: v1.ObjectNameFromAbsolutePath(
				filepath.Join(ws.Status.WorkspaceID, filename),
			),
			Namespace: ws.Namespace,
		},
		Spec: v1.KnowledgeFileSpec{
			FileName:         filename,
			KnowledgeSetName: knowledgeSetName,
			Approved:         &[]bool{true}[0],
			SizeInBytes:      int64(size),
		},
	}

	if err := req.Storage.Create(req.Context(), &file); err != nil && !apierrors.IsAlreadyExists(err) {
		_ = deleteFile(req.Context(), req, gClient, ws.Status.WorkspaceID, "")
		return err
	}

	return req.Write(convertKnowledgeFile(agentName, threadName, file))
}

func convertKnowledgeFile(agentName, threadName string, file v1.KnowledgeFile) types.KnowledgeFile {
	return types.KnowledgeFile{
		Metadata:               MetadataFrom(&file),
		FileName:               file.Spec.FileName,
		State:                  file.PublicState(),
		Error:                  file.Status.Error,
		Approved:               file.Spec.Approved,
		URL:                    file.Spec.URL,
		UpdatedAt:              file.Spec.UpdatedAt,
		Checksum:               file.Spec.Checksum,
		LastIngestionStartTime: types.NewTime(file.Status.LastIngestionStartTime.Time),
		LastIngestionEndTime:   types.NewTime(file.Status.LastIngestionEndTime.Time),
		AgentID:                agentName,
		ThreadID:               threadName,
		KnowledgeSetID:         file.Spec.KnowledgeSetName,
		KnowledgeSourceID:      file.Spec.KnowledgeSourceName,
		LastRunIDs:             file.Status.RunNames,
		SizeInBytes:            file.Spec.SizeInBytes,
	}
}

func compileFileNames(files []string, opts gptscript.ListFilesInWorkspaceOptions) []types.File {
	resp := make([]types.File, 0, len(files))
	for _, file := range files {
		resp = append(resp, convertFile(file, opts.Prefix))
	}

	return resp
}

func convertFile(file, prefix string) types.File {
	return types.File{
		Name: strings.TrimPrefix(file, prefix),
	}
}

func uploadFile(ctx context.Context, req api.Context, gClient *gptscript.GPTScript, workspaceName string) error {
	var ws v1.Workspace
	if err := req.Get(&ws, workspaceName); err != nil {
		return fmt.Errorf("failed to get workspace with id %s: %w", workspaceName, err)
	}

	_, err := uploadFileToWorkspace(ctx, req, gClient, ws.Status.WorkspaceID, "files/")
	return err
}

func getFileInWorkspace(ctx context.Context, req api.Context, gClient *gptscript.GPTScript, workspaceID, prefix string) error {
	file := req.PathValue("file")
	if file == "" {
		return fmt.Errorf("file path parameter is required")
	}

	data, err := gClient.ReadFileInWorkspace(ctx, prefix+file, gptscript.ReadFileInWorkspaceOptions{WorkspaceID: workspaceID})
	if err != nil {
		return fmt.Errorf("failed to get file %q to workspace %q: %w", file, workspaceID, err)
	}

	req.ResponseWriter.Header().Set("Content-Type", "application/octet-stream")
	req.ResponseWriter.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", file)) // make sure the file is downloaded with only the filename, not e.g. the dataset prefix
	_, err = req.ResponseWriter.Write(data)
	return err
}

func uploadFileToWorkspace(ctx context.Context, req api.Context, gClient *gptscript.GPTScript, workspaceID, prefix string, opts ...api.BodyOptions) (int, error) {
	file := req.PathValue("file")
	if file == "" {
		return 0, fmt.Errorf("file path parameter is required")
	}

	contents, err := req.Body(opts...)
	if err != nil {
		return 0, fmt.Errorf("failed to read request body: %w", err)
	}

	if err = gClient.WriteFileInWorkspace(ctx, prefix+file, contents, gptscript.WriteFileInWorkspaceOptions{WorkspaceID: workspaceID}); err != nil {
		return 0, fmt.Errorf("failed to upload file %q to workspace %q: %w", file, workspaceID, err)
	}

	req.WriteHeader(http.StatusCreated)

	return len(contents), nil
}

func deleteKnowledge(req api.Context, filename string, knowledgeSetName string) error {
	ws, err := getWorkspaceFromKnowledgeSet(req, knowledgeSetName)
	if err != nil {
		return err
	}

	return deleteKnowledgeFromWorkspace(req, filename, ws)
}

func deleteKnowledgeFromWorkspace(req api.Context, filename string, ws *v1.Workspace) error {
	if err := req.Delete(&v1.KnowledgeFile{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ws.Namespace,
			Name:      v1.ObjectNameFromAbsolutePath(filepath.Join(ws.Status.WorkspaceID, filename)),
		},
	}); err != nil {
		return err
	}

	req.WriteHeader(http.StatusNoContent)
	return nil
}

func deleteFile(ctx context.Context, req api.Context, gClient *gptscript.GPTScript, workspaceName, prefix string) error {
	var ws v1.Workspace
	if err := req.Get(&ws, workspaceName); err != nil {
		return err
	}

	return deleteFileFromWorkspaceID(ctx, req, gClient, ws.Status.WorkspaceID, prefix)
}

func deleteFileFromWorkspaceID(ctx context.Context, req api.Context, gClient *gptscript.GPTScript, workspaceID, prefix string) error {
	filename := req.PathValue("file")

	if err := gClient.DeleteFileInWorkspace(ctx, prefix+filename, gptscript.DeleteFileInWorkspaceOptions{WorkspaceID: workspaceID}); err != nil {
		return fmt.Errorf("failed to delete file %q from workspace %q: %w", filename, workspaceID, err)
	}

	req.WriteHeader(http.StatusNoContent)

	return nil
}

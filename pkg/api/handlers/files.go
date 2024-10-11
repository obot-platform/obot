package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/otto8-ai/otto8/apiclient/types"
	"github.com/otto8-ai/otto8/pkg/api"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.gptscript.ai/v1"
	"github.com/otto8-ai/otto8/pkg/storage/selectors"
	"github.com/otto8-ai/otto8/pkg/workspace"
	wclient "github.com/otto8-ai/workspace-provider/pkg/client"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func listFiles(ctx context.Context, req api.Context, wc *wclient.Client, workspaceName string) error {
	var ws v1.Workspace
	if err := req.Get(&ws, workspaceName); err != nil {
		return err
	}

	return listFileFromWorkspace(ctx, req, wc, ws)
}

func listFileFromWorkspace(ctx context.Context, req api.Context, wc *wclient.Client, ws v1.Workspace) error {
	files, err := wc.Ls(ctx, ws.Status.WorkspaceID)
	if err != nil {
		return fmt.Errorf("failed to list files in workspace %q: %w", ws.Status.WorkspaceID, err)
	}

	resp := make([]types.File, 0, len(files))
	for _, file := range files {
		resp = append(resp, convertFile(file))
	}

	return req.Write(types.FileList{Items: resp})
}

func getWorkspaceFromKnowledgeSet(req api.Context, knowledgeSetNames ...string) (ws v1.Workspace, ok bool, err error) {
	if len(knowledgeSetNames) == 0 {
		return ws, false, nil
	}

	var knowledgeSet v1.KnowledgeSet
	if err := req.Get(&knowledgeSet, knowledgeSetNames[0]); err != nil {
		return ws, false, err
	}

	if knowledgeSet.Status.WorkspaceName == "" {
		return ws, false, nil
	}

	err = req.Get(&ws, knowledgeSet.Status.WorkspaceName)
	return ws, true, err
}

func listKnowledgeFiles(req api.Context, knowledgeSetNames ...string) error {
	ws, ok, err := getWorkspaceFromKnowledgeSet(req, knowledgeSetNames...)
	if err != nil || !ok {
		return err
	}

	return listKnowledgeFilesFromWorkspace(req, ws)
}

func listKnowledgeFilesFromWorkspace(req api.Context, ws v1.Workspace) error {
	var files v1.KnowledgeFileList
	if err := req.Storage.List(req.Context(), &files, &client.ListOptions{
		FieldSelector: fields.SelectorFromSet(selectors.RemoveEmpty(map[string]string{
			"spec.workspaceName": ws.Name,
		})),
		Namespace: ws.Namespace,
	}); err != nil {
		return err
	}

	resp := make([]types.KnowledgeFile, 0, len(files.Items))
	for _, file := range files.Items {
		resp = append(resp, convertKnowledgeFile(file, ws))
	}

	return req.Write(types.KnowledgeFileList{Items: resp})
}

func uploadKnowledge(req api.Context, wc *wclient.Client, knowledgeSetNames ...string) error {
	ws, ok, err := getWorkspaceFromKnowledgeSet(req, knowledgeSetNames...)
	if err != nil || !ok {
		return err
	}

	return uploadKnowledgeToWorkspace(req, wc, ws)
}

func uploadKnowledgeToWorkspace(req api.Context, wc *wclient.Client, ws v1.Workspace) error {
	filename := req.PathValue("file")

	if err := uploadFileToWorkspace(req.Context(), req, wc, ws); err != nil {
		return err
	}

	file := v1.KnowledgeFile{
		ObjectMeta: metav1.ObjectMeta{
			Name: v1.ObjectNameFromAbsolutePath(
				filepath.Join(workspace.GetDir(ws.Status.WorkspaceID), filename),
			),
			Namespace: ws.Namespace,
		},
		Spec: v1.KnowledgeFileSpec{
			FileName:      filename,
			WorkspaceName: ws.Name,
		},
	}

	if err := req.Storage.Create(req.Context(), &file); err != nil && !apierrors.IsAlreadyExists(err) {
		_ = deleteFile(req.Context(), req, wc, ws.Status.WorkspaceID)
		return err
	}

	return req.Write(convertKnowledgeFile(file, ws))
}

func convertKnowledgeFile(file v1.KnowledgeFile, ws v1.Workspace) types.KnowledgeFile {
	return types.KnowledgeFile{
		Metadata:                  MetadataFrom(&file),
		FileName:                  file.Spec.FileName,
		AgentID:                   ws.Spec.AgentName,
		WorkflowID:                ws.Spec.WorkflowName,
		ThreadID:                  ws.Spec.ThreadName,
		IngestionStatus:           file.Status.IngestionStatus,
		FileDetails:               file.Status.FileDetails,
		RemoteKnowledgeSourceID:   file.Spec.RemoteKnowledgeSourceName,
		RemoteKnowledgeSourceType: file.Spec.RemoteKnowledgeSourceType,
		UploadID:                  file.Status.UploadID,
	}
}

func convertFile(file string) types.File {
	return types.File{
		Name: file,
	}
}

func uploadFile(ctx context.Context, req api.Context, wc *wclient.Client, workspaceName string) error {
	var ws v1.Workspace
	if err := req.Get(&ws, workspaceName); err != nil {
		return fmt.Errorf("failed to get workspace with id %s: %w", workspaceName, err)
	}

	return uploadFileToWorkspace(ctx, req, wc, ws)
}

func uploadFileToWorkspace(ctx context.Context, req api.Context, wc *wclient.Client, ws v1.Workspace) error {
	file := req.PathValue("file")
	if file == "" {
		return fmt.Errorf("file path parameter is required")
	}

	writer, err := wc.WriteFile(ctx, ws.Status.WorkspaceID, file)
	if err != nil {
		return fmt.Errorf("failed to upload file %q to workspace %q: %w", file, ws.Status.WorkspaceID, err)
	}

	_, err = io.Copy(writer, req.Request.Body)
	if err != nil {
		return fmt.Errorf("failed to write file %q to workspace %q: %w", file, ws.Status.WorkspaceID, err)
	}

	req.WriteHeader(http.StatusCreated)

	return nil
}

func deleteKnowledge(req api.Context, filename string, knowledgeSetNames ...string) error {
	ws, ok, err := getWorkspaceFromKnowledgeSet(req, knowledgeSetNames...)
	if err != nil || !ok {
		return err
	}

	return deleteKnowledgeFromWorkspace(req, filename, ws)
}

func deleteKnowledgeFromWorkspace(req api.Context, filename string, ws v1.Workspace) error {
	fileObjectName := v1.ObjectNameFromAbsolutePath(filepath.Join(workspace.GetDir(ws.Status.WorkspaceID), filename))

	if err := req.Storage.Delete(req.Context(), &v1.KnowledgeFile{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ws.Namespace,
			Name:      fileObjectName,
		},
	}); err != nil {
		var apiErr *apierrors.StatusError
		if errors.As(err, &apiErr) {
			apiErr.ErrStatus.Details.Name = filename
			apiErr.ErrStatus.Message = strings.ReplaceAll(apiErr.ErrStatus.Message, fileObjectName, filename)
		}
		return err
	}

	req.WriteHeader(http.StatusNoContent)
	return nil
}

func deleteFile(ctx context.Context, req api.Context, wc *wclient.Client, workspaceName string) error {
	var ws v1.Workspace
	if err := req.Get(&ws, workspaceName); err != nil {
		return err
	}

	return deleteFileFromWorkspaceID(ctx, req, wc, ws.Status.WorkspaceID)
}

func deleteFileFromWorkspaceID(ctx context.Context, req api.Context, wc *wclient.Client, workspaceID string) error {
	filename := req.PathValue("file")
	if err := wc.DeleteFile(ctx, workspaceID, filename); err != nil {
		return fmt.Errorf("failed to delete file %q from workspace %q: %w", filename, workspaceID, err)
	}

	req.WriteHeader(http.StatusNoContent)

	return nil
}

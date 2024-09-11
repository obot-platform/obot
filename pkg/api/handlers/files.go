package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/gptscript-ai/otto/pkg/api"
	"github.com/gptscript-ai/otto/pkg/api/types"
	wclient "github.com/thedadams/workspace-provider/pkg/client"
)

func listFiles(ctx context.Context, req api.Context, wc *wclient.Client, workspaceID string) error {
	files, err := wc.Ls(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to list files in workspace %q: %w", workspaceID, err)
	}

	return req.Write(types.FileList{Items: files})
}

func uploadFile(ctx context.Context, req api.Context, wc *wclient.Client, workspaceID string) error {
	file := req.Request.PathValue("file")
	if file == "" {
		return fmt.Errorf("file path parameter is required")
	}

	writer, err := wc.WriteFile(ctx, workspaceID, file)
	if err != nil {
		return fmt.Errorf("failed to upload file %q to workspace %q: %w", file, workspaceID, err)
	}

	_, err = io.Copy(writer, req.Request.Body)
	if err != nil {
		return fmt.Errorf("failed to write file %q to workspace %q: %w", file, workspaceID, err)
	}

	req.WriteHeader(http.StatusCreated)

	return nil
}

func deleteFile(ctx context.Context, req api.Context, wc *wclient.Client, workspaceID, filename string) error {
	if err := wc.DeleteFile(ctx, workspaceID, filename); err != nil {
		return fmt.Errorf("failed to delete file %q from workspace %q: %w", filename, workspaceID, err)
	}

	req.WriteHeader(http.StatusNoContent)

	return nil
}

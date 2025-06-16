package knowledgesource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type fileDetails struct {
	FilePath    string `json:"filePath,omitempty"`
	URL         string `json:"url,omitempty"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
	Checksum    string `json:"checksum,omitempty"`
	SizeInBytes int64  `json:"sizeInBytes,omitempty"`
}

type syncMetadata struct {
	Files  map[string]fileDetails `json:"files"`
	Status string                 `json:"status,omitempty"`
	State  map[string]any         `json:"state,omitempty"`
}

func getSharedWorkspaceID(ctx context.Context, c kclient.Client, thread *v1.Thread) string {
	if thread.Status.SharedWorkspaceName != "" {
		var workspace v1.Workspace
		if err := c.Get(ctx, router.Key(thread.Namespace, thread.Status.SharedWorkspaceName), &workspace); err == nil {
			return workspace.Status.WorkspaceID
		}
	}
	return ""
}

func (k *Handler) getMetadata(ctx context.Context, source *v1.KnowledgeSource, thread *v1.Thread, c kclient.Client) (result []v1.KnowledgeFile, _ *syncMetadata, _ error) {
	workspaceID := getSharedWorkspaceID(ctx, c, thread)
	if workspaceID == "" {
		return nil, nil, nil
	}

	data, err := k.gptClient.ReadFileInWorkspace(ctx, ".metadata.json", gptscript.ReadFileInWorkspaceOptions{
		WorkspaceID: workspaceID,
	})
	if errNotFound := new(gptscript.NotFoundInWorkspaceError); errors.As(err, &errNotFound) {
		return nil, nil, nil
	} else if err != nil {
		return nil, nil, fmt.Errorf("failed to read metadata.json: %w", err)
	}

	var output syncMetadata

	if err := json.Unmarshal(data, &output); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal metadata.json: %w", err)
	}

	for _, file := range output.Files {
		result = append(result, v1.KnowledgeFile{
			ObjectMeta: metav1.ObjectMeta{
				Name:       v1.ObjectNameFromAbsolutePath(filepath.Join(workspaceID, file.FilePath)),
				Namespace:  source.Namespace,
				Finalizers: []string{v1.KnowledgeFileFinalizer},
			},
			Spec: v1.KnowledgeFileSpec{
				KnowledgeSetName:    source.Spec.KnowledgeSetName,
				KnowledgeSourceName: source.Name,
				FileName:            file.FilePath,
				URL:                 file.URL,
				UpdatedAt:           file.UpdatedAt,
				Checksum:            file.Checksum,
				SizeInBytes:         file.SizeInBytes,
			},
		})
	}

	return result, &output, nil
}

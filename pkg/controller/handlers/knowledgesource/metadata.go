package knowledgesource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/gptscript-ai/go-gptscript"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.otto8.ai/v1"
	"github.com/otto8-ai/otto8/pkg/wait"
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

func (k *Handler) getWorkspaceID(ctx context.Context, c kclient.WithWatch, source *v1.KnowledgeSource) (string, error) {
	ws, err := wait.For(ctx, c, &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      source.Status.WorkspaceName,
			Namespace: source.Namespace,
		},
	}, func(ws *v1.Workspace) bool {
		return ws.Status.WorkspaceID != ""
	})
	if err != nil {
		return "", err
	}
	return ws.Status.WorkspaceID, nil
}

type syncMetadata struct {
	Files  map[string]fileDetails `json:"files"`
	Status string                 `json:"status,omitempty"`
	State  map[string]any         `json:"state,omitempty"`
}

func (k *Handler) getMetadata(ctx context.Context, source *v1.KnowledgeSource, thread *v1.Thread) (result []kclient.Object, _ *syncMetadata, _ error) {
	data, err := k.gptClient.ReadFileInWorkspace(ctx, ".metadata.json", gptscript.ReadFileInWorkspaceOptions{
		WorkspaceID: thread.Status.WorkspaceID,
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
		result = append(result, &v1.KnowledgeFile{
			ObjectMeta: metav1.ObjectMeta{
				Name:       v1.ObjectNameFromAbsolutePath(filepath.Join(thread.Status.WorkspaceID, file.FilePath)),
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

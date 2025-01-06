package knowledgesource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/gptscript-ai/go-gptscript"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func (k *Handler) getMetadata(ctx context.Context, source *v1.KnowledgeSource, thread *v1.Thread) (result []v1.KnowledgeFile, _ *syncMetadata, _ error) {
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
		result = append(result, v1.KnowledgeFile{
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

package knowledgesummary

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/controller/handlers/knowledgefile"
	"github.com/obot-platform/obot/pkg/gz"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	gptScript *gptscript.GPTScript
}

func NewHandler(gClient *gptscript.GPTScript) *Handler {
	return &Handler{
		gptScript: gClient,
	}
}

func (k *Handler) getFiles(req router.Request, thread *v1.Thread) ([]v1.KnowledgeFile, error) {
	var allFiles []v1.KnowledgeFile
	for _, setName := range thread.Status.KnowledgeSetNames {
		var files v1.KnowledgeFileList
		if err := req.Client.List(req.Ctx, &files, kclient.InNamespace(req.Namespace),
			&kclient.MatchingFields{"spec.knowledgeSetName": setName}); err != nil {
			return nil, err
		}
		allFiles = append(allFiles, files.Items...)
	}

	sort.Slice(allFiles, func(i, j int) bool {
		return allFiles[i].UID < allFiles[j].UID
	})

	return allFiles, nil
}

func toHash(files []v1.KnowledgeFile) string {
	digest := sha256.New()
	for _, file := range files {
		digest.Write([]byte(file.Name))
		digest.Write([]byte{'\x00'})
		digest.Write([]byte(file.Status.State))
		digest.Write([]byte{'\x00'})
		digest.Write([]byte(file.Status.Checksum))
		digest.Write([]byte{'\x00'})
	}
	return fmt.Sprintf("%x", digest.Sum(nil))
}

func (k *Handler) toAllContent(req router.Request, allFiles []v1.KnowledgeFile) ([]byte, error) {
	allContent := contentSummary{
		Files: make(map[string]summaryData),
	}

	for _, file := range allFiles {
		filename := knowledgefile.OutputFile(file.Spec.FileName)

		var knowledgeSet v1.KnowledgeSet
		if err := req.Get(&knowledgeSet, file.Namespace, file.Spec.KnowledgeSetName); err != nil {
			return nil, err
		}

		var workspace v1.Workspace
		if err := req.Get(&workspace, knowledgeSet.Namespace, knowledgeSet.Status.WorkspaceName); err != nil {
			return nil, err
		}

		if workspace.Status.WorkspaceID == "" {
			continue
		}

		data, err := k.gptScript.ReadFileInWorkspace(req.Ctx, filename, gptscript.ReadFileInWorkspaceOptions{
			WorkspaceID: workspace.Status.WorkspaceID,
		})
		if fErr := (*gptscript.NotFoundInWorkspaceError)(nil); err != nil && !errors.As(err, &fErr) {
			return nil, err
		}

		var content contentData
		if err := json.Unmarshal(data, &content); err == nil {
			if len(content.Documents) > 0 {
				var (
					combinedContent string
					partial         bool
				)

				// Accumulate content from documents until we have at least 3000 characters
				for _, doc := range content.Documents {
					if doc.Content == "" {
						continue
					}
					combinedContent += doc.Content
					if len(combinedContent) >= 3000 {
						break
					}
				}

				if combinedContent != "" {
					if len(combinedContent) > 3000 {
						combinedContent = combinedContent[:3000]
						partial = true
					}
					allContent.Files[file.Spec.FileName] = summaryData{
						Content: combinedContent,
						Partial: partial,
					}
				}
			}
		}
	}

	out, err := gz.Compress(allContent)
	if err != nil {
		return nil, fmt.Errorf("failed to compress content: %w", err)
	}
	return out, nil
}

func (k *Handler) Summarize(req router.Request, _ router.Response) error {
	thread := req.Object.(*v1.Thread)

	allFiles, err := k.getFiles(req, thread)
	if err != nil {
		return err
	}

	var summary v1.KnowledgeSummary
	if err := req.Get(&summary, thread.Namespace, thread.Name); apierror.IsNotFound(err) {
		if len(allFiles) == 0 {
			return nil
		}
	} else if err != nil {
		return err
	}

	if len(allFiles) == 0 {
		return req.Delete(&summary)
	}

	hash := toHash(allFiles)
	if summary.Spec.ContentHash == hash {
		return nil
	}

	out, err := k.toAllContent(req, allFiles)
	if err != nil {
		return err
	}

	if summary.Name == "" {
		return req.Client.Create(req.Ctx, &v1.KnowledgeSummary{
			ObjectMeta: metav1.ObjectMeta{
				Name:      thread.Name,
				Namespace: thread.Namespace,
			},
			Spec: v1.KnowledgeSummarySpec{
				ThreadName:  thread.Name,
				ContentHash: hash,
				Summary:     out,
			},
		})
	}

	summary.Spec.ContentHash = hash
	summary.Spec.Summary = out
	return req.Client.Update(req.Ctx, &summary)
}

type contentSummary struct {
	Files map[string]summaryData `json:"files,omitempty"`
}

type summaryData struct {
	Content string `json:"content"`
	Partial bool   `json:"partial"`
}

type contentData struct {
	Documents []documentData `json:"documents,omitempty"`
}

type documentData struct {
	Content string `json:"content,omitempty"`
}

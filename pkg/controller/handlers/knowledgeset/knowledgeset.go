package knowledgeset

import (
	"strings"

	"github.com/acorn-io/baaah/pkg/router"
	"github.com/otto8-ai/otto8/pkg/aihelper"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.gptscript.ai/v1"
	"github.com/otto8-ai/otto8/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	aiHelper *aihelper.AIHelper
}

func New(aihelper *aihelper.AIHelper) *Handler {
	return &Handler{aiHelper: aihelper}
}

func (h *Handler) GenerateDataDescription(req router.Request, resp router.Response) error {
	var (
		ks = req.Object.(*v1.KnowledgeSet)
		ws v1.Workspace
	)

	if ks.Status.WorkspaceName == "" || ks.Spec.Manifest.DataDescription != "" {
		return nil
	}

	if err := req.Client.Get(req.Ctx, router.Key(ks.Namespace, ks.Status.WorkspaceName), &ws); apierrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return err
	}

	// Check if in sync already
	if ks.Status.ObservedIngestionGeneration == ws.Status.IngestionGeneration {
		return nil
	}

	// Ignore if running
	if ws.Status.IngestionRunName != "" {
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

	ks.Status.ObservedIngestionGeneration = ws.Status.IngestionGeneration
	return h.aiHelper.GenerateObject(req.Ctx, &ks.Status.SuggestedDataDescription, generatePrompt(files), "")
}

func generatePrompt(files v1.KnowledgeFileList) string {
	var (
		prompt    string
		fileNames = make([]string, 0, len(files.Items))
	)

	for _, file := range files.Items {
		fileNames = append(fileNames, "- "+file.Spec.FileName)
	}

	fileText := strings.Join(fileNames, "\n")
	if len(fileText) > 50000 {
		fileText = fileText[:50000]
	}

	prompt = "The following files are in this knowledge set:\n" + fileText
	prompt += "\n\nGenerate a 50 word description of the data in the knowledge set that would help a" +
		" reader understand why they might want to search this knowledge set. Be precise and concise."
	return prompt
}

func (h *Handler) CreateKnowledgeSet(req router.Request, resp router.Response) error {
	ks := req.Object.(*v1.KnowledgeSet)

	if ks.Status.WorkspaceName != "" {
		return nil
	}

	ws := &v1.Workspace{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.WorkspacePrefix,
			Namespace:    ks.Namespace,
		},
		Spec: v1.WorkspaceSpec{
			KnowledgeSetName: ks.Name,
			IsKnowledge:      true,
		},
		Status: v1.WorkspaceStatus{},
	}

	if err := req.Client.Create(req.Ctx, ws); err != nil {
		return err
	}

	ks.Status.WorkspaceName = ws.Name
	return req.Client.Status().Update(req.Ctx, ks)
}

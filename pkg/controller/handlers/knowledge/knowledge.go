package knowledge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/acorn-io/baaah/pkg/apply"
	"github.com/acorn-io/baaah/pkg/router"
	"github.com/gptscript-ai/otto/apiclient/types"
	"github.com/gptscript-ai/otto/pkg/invoke"
	"github.com/gptscript-ai/otto/pkg/knowledge"
	"github.com/gptscript-ai/otto/pkg/mvl"
	v1 "github.com/gptscript-ai/otto/pkg/storage/apis/otto.gptscript.ai/v1"
	"github.com/gptscript-ai/otto/pkg/storage/selectors"
	"github.com/gptscript-ai/otto/pkg/system"
	wclient "github.com/thedadams/workspace-provider/pkg/client"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	workspaceClient   *wclient.Client
	ingester          *knowledge.Ingester
	workspaceProvider string
}

func New(wc *wclient.Client, ingester *knowledge.Ingester, wp string) *Handler {
	return &Handler{
		workspaceClient:   wc,
		ingester:          ingester,
		workspaceProvider: wp,
	}
}

func (a *Handler) IngestKnowledge(req router.Request, _ router.Response) error {
	ws := req.Object.(*v1.Workspace)
	if !ws.Spec.IsKnowledge {
		return nil
	}

	if ws.Status.IngestionRunName != "" {
		// Check to see if the run is still running
		var run v1.Run
		if err := req.Get(&run, ws.Namespace, ws.Status.IngestionRunName); err != nil && !apierrors.IsNotFound(err) {
			return err
		} else if err == nil && !run.Status.State.IsTerminal() {
			// The run hasn't completed, so don't create another one.
			return nil
		}
	}

	ws.Status.IngestionRunName = ""

	// Get the reIngestRequests for this workspace
	var reIngestRequests v1.IngestKnowledgeRequestList
	if err := req.List(&reIngestRequests, &kclient.ListOptions{
		FieldSelector: fields.SelectorFromSet(selectors.RemoveEmpty(map[string]string{
			"spec.workspaceName": ws.Name,
		})),
		Namespace: ws.Namespace,
	}); err != nil {
		return err
	}

	if len(reIngestRequests.Items) == 0 {
		return nil
	}

	var needsIngestion, hasKnowledge bool
	for _, r := range reIngestRequests.Items {
		if ws.Status.LastIngestionRunStarted.Before(&r.CreationTimestamp) {
			needsIngestion = true
			if r.Spec.HasKnowledge {
				hasKnowledge = true
				break
			}
		}
	}

	if !needsIngestion {
		return nil
	}

	if !hasKnowledge {
		ws.Status.HasKnowledge = false
		// Set the last ingestion time so that the re-ingestion requests get cleaned up.
		ws.Status.LastIngestionRunStarted = metav1.Now()
		return nil
	}

	var (
		run *invoke.Response
		err error
	)
	run, err = a.ingester.IngestKnowledge(req.Ctx, ws.Spec.AgentName, ws.GetNamespace(), ws.Status.WorkspaceID)
	if err != nil {
		return err
	}

	go compileFileStatuses(req.Ctx, req.Client, ws, run, mvl.Package())

	ws.Status.IngestionRunName = run.Run.Name
	ws.Status.LastIngestionRunStarted = metav1.Now()
	ws.Status.HasKnowledge = true
	return nil
}

func compileFileStatuses(ctx context.Context, client kclient.Client, ws *v1.Workspace, run *invoke.Response, logger mvl.Logger) {
	for e := range run.Events {
		for _, line := range strings.Split(e.Content, "\n") {
			if line == "" || line[0] != '{' || line[len(line)-1] != '}' {
				continue
			}
			var ingestionStatus types.IngestionStatus
			if err := json.Unmarshal([]byte(line), &ingestionStatus); err != nil {
				logger.Errorf("failed to unmarshal event: %s", err)
			}

			if ingestionStatus.Filepath == "" {
				// Not a file status log.
				continue
			}

			var file v1.KnowledgeFile
			if err := client.Get(ctx, router.Key(ws.GetNamespace(), v1.ObjectNameFromAbsolutePath(ingestionStatus.Filepath)), &file); apierrors.IsNotFound(err) {
				// Don't error if the file is not found. It may have been deleted, and the next ingestion will pick that up.
				continue
			} else if err != nil {
				logger.Errorf("failed to get knowledge file: %s", err)
			}

			file.Status.IngestionStatus = ingestionStatus
			if err := client.Status().Update(ctx, &file); err != nil {
				logger.Errorf("failed to update knowledge file: %s", err)
			}
		}
	}
}

func (a *Handler) CleanupFile(req router.Request, resp router.Response) error {
	kFile := req.Object.(*v1.KnowledgeFile)

	var ws v1.Workspace
	if err := req.Get(&ws, kFile.Namespace, kFile.Spec.WorkspaceName); apierrors.IsNotFound(err) {
		// If the workspace object is not found, then the workspaces will be deleted and nothing needs to happen here.
		return nil
	} else if err != nil {
		return err
	}

	if err := a.workspaceClient.DeleteFile(req.Ctx, ws.Status.WorkspaceID, kFile.Spec.FileName); err != nil {
		if errors.As(err, new(wclient.FileNotFoundError)) {
			// It is important to return nil here and not move forward because when bulk deleting files from a remote provider
			// (like OneDrive), the connector will remove the files from the local disk and the controller will remove the
			// KnowledgeFile objects. We don't want to kick off (possibly) numerous ingestion runs.
			return nil
		}
		return err
	}

	files, err := a.workspaceClient.Ls(req.Ctx, ws.Status.WorkspaceID)
	if err != nil {
		return fmt.Errorf("failed to list files in workspace %s: %w", ws.Status.WorkspaceID, err)
	}

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
				HasKnowledge:  len(files) > 0,
			},
		},
	)

	return nil
}

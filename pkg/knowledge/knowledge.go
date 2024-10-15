package knowledge

import (
	"context"

	"github.com/otto8-ai/otto8/pkg/invoke"
	"github.com/otto8-ai/otto8/pkg/system"
	"github.com/otto8-ai/otto8/pkg/workspace"
)

type Ingester struct {
	invoker *invoke.Invoker
}

func NewIngester(invoker *invoke.Invoker) *Ingester {
	return &Ingester{
		invoker: invoker,
	}
}

func (i *Ingester) IngestKnowledge(ctx context.Context, namespace, knowledgeSetName, workspaceID string) (*invoke.Response, error) {
	return i.invoker.SystemAction(
		ctx,
		"ingest-",
		namespace,
		system.KnowledgeIngestTool,
		workspace.GetDir(workspaceID),
		// Below are environment variables used by the ingest tool
		"GPTSCRIPT_DATASET="+knowledgeSetName,
		"KNOW_JSON=true",
	)
}

func (i *Ingester) DeleteKnowledgeFiles(ctx context.Context, namespace, knowledgeFilePath string) (*invoke.Response, error) {
	return i.invoker.SystemAction(
		ctx,
		"ingest-delete-file-",
		namespace,
		system.KnowledgeDeleteFileTool,
		knowledgeFilePath,
	)

}

func (i *Ingester) DeleteKnowledge(ctx context.Context, namespace, knowledgeSetName string) (*invoke.Response, error) {
	return i.invoker.SystemAction(
		ctx,
		"ingest-delete-",
		namespace,
		system.KnowledgeDeleteTool,
		knowledgeSetName,
	)
}

package controller

import (
	"github.com/acorn-io/baaah/pkg/router"
	"github.com/gptscript-ai/otto/pkg/controller/handlers/agents"
	"github.com/gptscript-ai/otto/pkg/controller/handlers/cleanup"
	knowledgehandler "github.com/gptscript-ai/otto/pkg/controller/handlers/knowledge"
	"github.com/gptscript-ai/otto/pkg/controller/handlers/reference"
	"github.com/gptscript-ai/otto/pkg/controller/handlers/runs"
	"github.com/gptscript-ai/otto/pkg/controller/handlers/threads"
	"github.com/gptscript-ai/otto/pkg/controller/handlers/toolreference"
	"github.com/gptscript-ai/otto/pkg/controller/handlers/uploads"
	"github.com/gptscript-ai/otto/pkg/controller/handlers/webhookexecution"
	"github.com/gptscript-ai/otto/pkg/controller/handlers/workflow"
	"github.com/gptscript-ai/otto/pkg/controller/handlers/workflowexecution"
	"github.com/gptscript-ai/otto/pkg/controller/handlers/workflowstep"
	"github.com/gptscript-ai/otto/pkg/controller/handlers/workspace"
	"github.com/gptscript-ai/otto/pkg/knowledge"
	"github.com/gptscript-ai/otto/pkg/services"
	v1 "github.com/gptscript-ai/otto/pkg/storage/apis/otto.gptscript.ai/v1"
)

func routes(root *router.Router, svcs *services.Services) error {
	workflowExecution := workflowexecution.New(svcs.WorkspaceClient, svcs.Invoker)
	workflowStep := workflowstep.New(svcs.Invoker)
	ingester := knowledge.NewIngester(svcs.Invoker, svcs.SystemTools[services.SystemToolKnowledge])
	agents := agents.New(svcs.AIHelper)
	toolRef := toolreference.New(svcs.GPTClient)
	threads := threads.New(svcs.AIHelper)
	workspace := workspace.New(svcs.WorkspaceClient, "directory")
	knowledge := knowledgehandler.New(svcs.WorkspaceClient, ingester, "directory")
	uploads := uploads.New(svcs.Invoker, svcs.WorkspaceClient, "directory", svcs.SystemTools[services.SystemToolOneDrive])
	runs := runs.New(svcs.Invoker)
	webhookExecutions := webhookexecution.New(svcs.WorkspaceClient, svcs.Invoker)

	// Runs
	root.Type(&v1.Run{}).FinalizeFunc(v1.RunFinalizer, runs.DeleteRunState)
	root.Type(&v1.Run{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.Run{}).HandlerFunc(runs.Resume)

	// Threads
	root.Type(&v1.Thread{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.Thread{}).HandlerFunc(threads.Description)
	root.Type(&v1.Thread{}).HandlerFunc(threads.CreateWorkspaces)

	// Workflows
	root.Type(&v1.Workflow{}).HandlerFunc(workflow.WorkspaceObjects)
	root.Type(&v1.Workflow{}).HandlerFunc(workflow.EnsureIDs)

	// WorkflowExecutions
	root.Type(&v1.WorkflowExecution{}).FinalizeFunc(v1.WorkflowExecutionFinalizer, workflowExecution.Cleanup)
	root.Type(&v1.WorkflowExecution{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.WorkflowExecution{}).HandlerFunc(workflowExecution.Run)

	// WorkflowSteps
	steps := root.Type(&v1.WorkflowStep{})
	steps.HandlerFunc(workflowStep.SetRunning)
	steps.HandlerFunc(cleanup.Cleanup)

	running := steps.Middleware(workflowstep.Running)
	running.HandlerFunc(workflowStep.RunInvoke)
	running.HandlerFunc(workflowStep.RunIf)
	running.HandlerFunc(workflowStep.RunWhile)
	steps.HandlerFunc(workflowStep.RunSubflow)

	// Agents
	root.Type(&v1.Agent{}).HandlerFunc(agents.Suggestion)
	root.Type(&v1.Agent{}).HandlerFunc(agents.WorkspaceObjects)

	// Uploads
	root.Type(&v1.OneDriveLinks{}).HandlerFunc(uploads.CreateThread)
	root.Type(&v1.OneDriveLinks{}).HandlerFunc(uploads.RunUpload)
	root.Type(&v1.OneDriveLinks{}).HandlerFunc(uploads.HandleUploadRun)
	root.Type(&v1.OneDriveLinks{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.OneDriveLinks{}).FinalizeFunc(v1.OneDriveLinksFinalizer, uploads.Cleanup)

	// ReSync requests
	root.Type(&v1.SyncUploadRequest{}).HandlerFunc(uploads.CleanupSyncRequests)

	// ToolReference
	root.Type(&v1.ToolReference{}).HandlerFunc(toolRef.Populate)

	// Reference
	root.Type(&v1.Reference{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.Agent{}).HandlerFunc(reference.AssociateWithReference)
	root.Type(&v1.Workflow{}).HandlerFunc(reference.AssociateWithReference)

	// Knowledge files
	root.Type(&v1.KnowledgeFile{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.KnowledgeFile{}).FinalizeFunc(v1.KnowledgeFileFinalizer, knowledge.CleanupFile)

	// ReIngest requests
	root.Type(&v1.IngestKnowledgeRequest{}).HandlerFunc(knowledge.CleanupIngestRequests)

	// Workspaces
	root.Type(&v1.Workspace{}).FinalizeFunc(v1.WorkspaceFinalizer, workspace.RemoveWorkspace)
	root.Type(&v1.Workspace{}).HandlerFunc(workspace.CreateWorkspace)
	root.Type(&v1.Workspace{}).HandlerFunc(knowledge.IngestKnowledge)
	root.Type(&v1.Workspace{}).HandlerFunc(cleanup.Cleanup)

	// Webhooks
	root.Type(&v1.Webhook{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.Webhook{}).HandlerFunc(reference.AssociateWebhookWithReference)
	root.Type(&v1.WebhookReference{}).HandlerFunc(reference.Cleanup)

	// Webhook executions
	root.Type(&v1.WebhookExecution{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.WebhookExecution{}).HandlerFunc(webhookExecutions.Run)

	return nil
}

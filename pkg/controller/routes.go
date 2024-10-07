package controller

import (
	"github.com/acorn-io/baaah/pkg/handlers"
	"github.com/gptscript-ai/otto/pkg/controller/handlers/agents"
	"github.com/gptscript-ai/otto/pkg/controller/handlers/cleanup"
	"github.com/gptscript-ai/otto/pkg/controller/handlers/cronjob"
	knowledgehandler "github.com/gptscript-ai/otto/pkg/controller/handlers/knowledge"
	"github.com/gptscript-ai/otto/pkg/controller/handlers/reference"
	"github.com/gptscript-ai/otto/pkg/controller/handlers/runs"
	"github.com/gptscript-ai/otto/pkg/controller/handlers/threads"
	"github.com/gptscript-ai/otto/pkg/controller/handlers/toolreference"
	"github.com/gptscript-ai/otto/pkg/controller/handlers/uploads"
	"github.com/gptscript-ai/otto/pkg/controller/handlers/webhook"
	"github.com/gptscript-ai/otto/pkg/controller/handlers/workflow"
	"github.com/gptscript-ai/otto/pkg/controller/handlers/workflowexecution"
	"github.com/gptscript-ai/otto/pkg/controller/handlers/workflowstep"
	"github.com/gptscript-ai/otto/pkg/controller/handlers/workspace"
	"github.com/gptscript-ai/otto/pkg/knowledge"
	"github.com/gptscript-ai/otto/pkg/services"
	v1 "github.com/gptscript-ai/otto/pkg/storage/apis/otto.gptscript.ai/v1"
)

func (c *Controller) setupRoutes() error {
	root := c.router

	workflowExecution := workflowexecution.New(c.services.WorkspaceClient, c.services.Invoker)
	workflowStep := workflowstep.New(c.services.Invoker)
	ingester := knowledge.NewIngester(c.services.Invoker, c.services.SystemTools[services.SystemToolKnowledge])
	agents := agents.New(c.services.AIHelper)
	toolRef := toolreference.New(c.services.GPTClient, c.services.ToolRegistryURL)
	threads := threads.New(c.services.AIHelper)
	workspace := workspace.New(c.services.WorkspaceClient, "directory")
	knowledge := knowledgehandler.New(c.services.WorkspaceClient, ingester, "directory")
	uploads := uploads.New(c.services.Invoker, c.services.WorkspaceClient, "directory", c.services.SystemTools[services.SystemToolOneDrive], c.services.SystemTools[services.SystemToolNotion], c.services.SystemTools[services.SystemToolWebsite])
	runs := runs.New(c.services.Invoker)
	webHooks := webhook.New()
	cronJobs := cronjob.New()

	// Runs
	root.Type(&v1.Run{}).FinalizeFunc(v1.RunFinalizer, runs.DeleteRunState)
	root.Type(&v1.Run{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.Run{}).HandlerFunc(runs.Resume)

	// Threads
	root.Type(&v1.Thread{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.Thread{}).HandlerFunc(threads.Description)
	root.Type(&v1.Thread{}).HandlerFunc(threads.CreateWorkspaces)
	root.Type(&v1.Thread{}).HandlerFunc(threads.WorkflowState)

	// Workflows
	root.Type(&v1.Workflow{}).HandlerFunc(workflow.WorkspaceObjects)
	root.Type(&v1.Workflow{}).HandlerFunc(workflow.EnsureIDs)

	// WorkflowExecutions
	root.Type(&v1.WorkflowExecution{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.WorkflowExecution{}).HandlerFunc(workflowExecution.Run)

	// WorkflowSteps
	steps := root.Type(&v1.WorkflowStep{})
	steps.HandlerFunc(cleanup.Cleanup)
	steps.HandlerFunc(handlers.GCOrphans)

	running := steps.Middleware(workflowStep.Preconditions)
	running.HandlerFunc(workflowStep.RunInvoke)
	running.HandlerFunc(workflowStep.RunIf)
	running.HandlerFunc(workflowStep.RunWhile)
	steps.HandlerFunc(workflowStep.RunSubflow)

	// Agents
	root.Type(&v1.Agent{}).HandlerFunc(agents.Suggestion)
	root.Type(&v1.Agent{}).HandlerFunc(agents.WorkspaceObjects)

	// Uploads
	root.Type(&v1.RemoteKnowledgeSource{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.RemoteKnowledgeSource{}).HandlerFunc(uploads.CreateThread)
	root.Type(&v1.RemoteKnowledgeSource{}).HandlerFunc(uploads.RunUpload)
	root.Type(&v1.RemoteKnowledgeSource{}).HandlerFunc(uploads.HandleUploadRun)
	root.Type(&v1.RemoteKnowledgeSource{}).FinalizeFunc(v1.RemoteKnowledgeSourceFinalizer, uploads.Cleanup)

	// ReSync requests
	root.Type(&v1.SyncUploadRequest{}).HandlerFunc(uploads.CleanupSyncRequests)

	// ToolReference
	root.Type(&v1.ToolReference{}).HandlerFunc(toolRef.Populate)

	// Reference
	root.Type(&v1.Reference{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.Agent{}).HandlerFunc(reference.AssociateWithReference)
	root.Type(&v1.Workflow{}).HandlerFunc(reference.AssociateWithReference)
	root.Type(&v1.Reference{}).HandlerFunc(reference.Cleanup)

	// Knowledge files
	root.Type(&v1.KnowledgeFile{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.KnowledgeFile{}).FinalizeFunc(v1.KnowledgeFileFinalizer, knowledge.CleanupFile)

	// ReIngest requests
	root.Type(&v1.IngestKnowledgeRequest{}).HandlerFunc(knowledge.CleanupIngestRequests)

	// Workspaces
	root.Type(&v1.Workspace{}).FinalizeFunc(v1.WorkspaceFinalizer, workspace.RemoveWorkspace)
	root.Type(&v1.Workspace{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.Workspace{}).HandlerFunc(workspace.CreateWorkspace)
	root.Type(&v1.Workspace{}).HandlerFunc(knowledge.IngestKnowledge)

	// Webhooks
	root.Type(&v1.Webhook{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.Webhook{}).HandlerFunc(reference.AssociateWebhookWithReference)
	root.Type(&v1.Webhook{}).HandlerFunc(webHooks.AssignRefName)
	root.Type(&v1.Webhook{}).HandlerFunc(webHooks.SetSuccessRunTime)

	// Webhook references
	root.Type(&v1.WebhookReference{}).HandlerFunc(reference.CleanupWebhook)

	// Cronjobs
	root.Type(&v1.CronJob{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.CronJob{}).HandlerFunc(cronJobs.SetSuccessRunTime)
	root.Type(&v1.CronJob{}).HandlerFunc(cronJobs.Run)

	// OAuthApps
	root.Type(&v1.OAuthApp{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.OAuthApp{}).HandlerFunc(reference.AssociateOAuthAppWithReference)

	// OAuthAppReferences
	root.Type(&v1.OAuthAppReference{}).HandlerFunc(reference.CleanupOAuthApp)

	c.toolRefHandler = toolRef
	return nil
}

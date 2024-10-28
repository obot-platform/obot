package controller

import (
	"github.com/acorn-io/baaah/pkg/handlers"
	"github.com/otto8-ai/otto8/pkg/controller/handlers/agents"
	"github.com/otto8-ai/otto8/pkg/controller/handlers/cleanup"
	"github.com/otto8-ai/otto8/pkg/controller/handlers/cronjob"
	knowledgehandler "github.com/otto8-ai/otto8/pkg/controller/handlers/knowledge"
	"github.com/otto8-ai/otto8/pkg/controller/handlers/knowledgeset"
	"github.com/otto8-ai/otto8/pkg/controller/handlers/oauthapp"
	"github.com/otto8-ai/otto8/pkg/controller/handlers/reference"
	"github.com/otto8-ai/otto8/pkg/controller/handlers/runs"
	"github.com/otto8-ai/otto8/pkg/controller/handlers/threads"
	"github.com/otto8-ai/otto8/pkg/controller/handlers/toolreference"
	"github.com/otto8-ai/otto8/pkg/controller/handlers/uploads"
	"github.com/otto8-ai/otto8/pkg/controller/handlers/webhook"
	"github.com/otto8-ai/otto8/pkg/controller/handlers/workflow"
	"github.com/otto8-ai/otto8/pkg/controller/handlers/workflowexecution"
	"github.com/otto8-ai/otto8/pkg/controller/handlers/workflowstep"
	"github.com/otto8-ai/otto8/pkg/controller/handlers/workspace"
	"github.com/otto8-ai/otto8/pkg/knowledge"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.gptscript.ai/v1"
)

func (c *Controller) setupRoutes() error {
	root := c.router

	workflowExecution := workflowexecution.New(c.services.Invoker)
	workflowStep := workflowstep.New(c.services.Invoker)
	ingester := knowledge.NewIngester(c.services.Invoker)
	toolRef := toolreference.New(c.services.GPTClient, c.services.ToolRegistryURL)
	workspace := workspace.New(c.services.GPTClient, c.services.WorkspaceProviderType)
	knowledge := knowledgehandler.New(c.services.GPTClient, ingester, c.services.Events)
	knowledgeset := knowledgeset.New(c.services.AIHelper)
	uploads := uploads.New(c.services.Invoker, c.services.GPTClient, "directory")
	runs := runs.New(c.services.Invoker)
	webHooks := webhook.New()
	cronJobs := cronjob.New()
	oauthLogins := oauthapp.NewLogin(c.services.Invoker)

	// Runs
	root.Type(&v1.Run{}).FinalizeFunc(v1.RunFinalizer, runs.DeleteRunState)
	root.Type(&v1.Run{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.Run{}).HandlerFunc(runs.Resume)

	// Threads
	root.Type(&v1.Thread{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.Thread{}).HandlerFunc(threads.CreateWorkspaces)
	root.Type(&v1.Thread{}).HandlerFunc(threads.CreateKnowledgeSet)
	root.Type(&v1.Thread{}).HandlerFunc(threads.WorkflowState)
	root.Type(&v1.Thread{}).HandlerFunc(threads.PurgeSystemThread)

	// Workflows
	root.Type(&v1.Workflow{}).HandlerFunc(workflow.WorkspaceObjects)
	root.Type(&v1.Workflow{}).HandlerFunc(workflow.EnsureIDs)

	// WorkflowExecutions
	root.Type(&v1.WorkflowExecution{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.WorkflowExecution{}).HandlerFunc(workflowExecution.Run)

	// Agents
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

	// Workspaces
	root.Type(&v1.Workspace{}).FinalizeFunc(v1.WorkspaceFinalizer, workspace.RemoveWorkspace)
	root.Type(&v1.Workspace{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.Workspace{}).HandlerFunc(workspace.CreateWorkspace)
	root.Type(&v1.Workspace{}).HandlerFunc(knowledge.IngestKnowledge)
	root.Type(&v1.Workspace{}).HandlerFunc(knowledge.UpdateFileStatus)
	root.Type(&v1.Workspace{}).HandlerFunc(knowledge.UpdateIngestionError)

	// KnowledgeSets
	root.Type(&v1.KnowledgeSet{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.KnowledgeSet{}).HandlerFunc(knowledgeset.GenerateDataDescription)
	root.Type(&v1.KnowledgeSet{}).HandlerFunc(knowledgeset.CreateWorkspace)

	// Webhooks
	root.Type(&v1.Webhook{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.Webhook{}).HandlerFunc(reference.AssociateWebhookWithReference)
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

	// OAuthAppLogins
	root.Type(&v1.OAuthAppLogin{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.OAuthAppLogin{}).HandlerFunc(oauthLogins.RunTool)

	// WorkflowSteps
	steps := root.Type(&v1.WorkflowStep{})
	steps.HandlerFunc(cleanup.Cleanup)
	steps.HandlerFunc(handlers.GCOrphans)

	running := steps.Middleware(workflowStep.Preconditions)
	running.HandlerFunc(workflowStep.RunInvoke)
	running.HandlerFunc(workflowStep.RunIf)
	running.HandlerFunc(workflowStep.RunWhile)
	steps.HandlerFunc(workflowStep.RunSubflow)

	c.toolRefHandler = toolRef
	return nil
}

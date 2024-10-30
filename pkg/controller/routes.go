package controller

import (
	"github.com/acorn-io/baaah/pkg/handlers"
	"github.com/otto8-ai/otto8/pkg/controller/handlers/agents"
	"github.com/otto8-ai/otto8/pkg/controller/handlers/cleanup"
	"github.com/otto8-ai/otto8/pkg/controller/handlers/cronjob"
	"github.com/otto8-ai/otto8/pkg/controller/handlers/knowledgefile"
	"github.com/otto8-ai/otto8/pkg/controller/handlers/knowledgeset"
	"github.com/otto8-ai/otto8/pkg/controller/handlers/knowledgesource"
	"github.com/otto8-ai/otto8/pkg/controller/handlers/oauthapp"
	"github.com/otto8-ai/otto8/pkg/controller/handlers/reference"
	"github.com/otto8-ai/otto8/pkg/controller/handlers/runs"
	"github.com/otto8-ai/otto8/pkg/controller/handlers/threads"
	"github.com/otto8-ai/otto8/pkg/controller/handlers/toolreference"
	"github.com/otto8-ai/otto8/pkg/controller/handlers/webhook"
	"github.com/otto8-ai/otto8/pkg/controller/handlers/workflow"
	"github.com/otto8-ai/otto8/pkg/controller/handlers/workflowexecution"
	"github.com/otto8-ai/otto8/pkg/controller/handlers/workflowstep"
	"github.com/otto8-ai/otto8/pkg/controller/handlers/workspace"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.gptscript.ai/v1"
)

func (c *Controller) setupRoutes() error {
	root := c.router

	workflowExecution := workflowexecution.New(c.services.Invoker)
	workflowStep := workflowstep.New(c.services.Invoker)
	toolRef := toolreference.New(c.services.GPTClient, c.services.ToolRegistryURL)
	workspace := workspace.New(c.services.GPTClient, c.services.WorkspaceProviderType)
	knowledgeset := knowledgeset.New(c.services.AIHelper, c.services.Invoker)
	knowledgesource := knowledgesource.NewHandler(c.services.Invoker, c.services.GPTClient)
	knowledgefile := knowledgefile.New(c.services.Invoker, c.services.GPTClient)
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

	// Workflows
	root.Type(&v1.Workflow{}).HandlerFunc(workflow.WorkspaceObjects)
	root.Type(&v1.Workflow{}).HandlerFunc(workflow.EnsureIDs)

	// WorkflowExecutions
	root.Type(&v1.WorkflowExecution{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.WorkflowExecution{}).HandlerFunc(workflowExecution.Run)

	// Agents
	root.Type(&v1.Agent{}).HandlerFunc(agents.CreateWorkspaceAndKnowledgeSet)

	// Uploads
	root.Type(&v1.KnowledgeSource{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.KnowledgeSource{}).FinalizeFunc(v1.KnowledgeSourceFinalizer, knowledgesource.Cleanup)
	root.Type(&v1.KnowledgeSource{}).HandlerFunc(knowledgesource.Reschedule)
	root.Type(&v1.KnowledgeSource{}).HandlerFunc(knowledgesource.Sync)
	root.Type(&v1.KnowledgeSource{}).HandlerFunc(knowledgesource.BackPopulateAuthStatus)

	// ToolReference
	root.Type(&v1.ToolReference{}).HandlerFunc(toolRef.Populate)

	// Reference
	root.Type(&v1.Reference{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.Agent{}).HandlerFunc(reference.AssociateWithReference)
	root.Type(&v1.Workflow{}).HandlerFunc(reference.AssociateWithReference)
	root.Type(&v1.Reference{}).HandlerFunc(reference.Cleanup)

	// Knowledge files
	root.Type(&v1.KnowledgeFile{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.KnowledgeFile{}).FinalizeFunc(v1.KnowledgeFileFinalizer, knowledgefile.Cleanup)
	root.Type(&v1.KnowledgeFile{}).HandlerFunc(knowledgefile.IngestFile)

	// Workspaces
	root.Type(&v1.Workspace{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.Workspace{}).FinalizeFunc(v1.WorkspaceFinalizer, workspace.RemoveWorkspace)
	root.Type(&v1.Workspace{}).HandlerFunc(workspace.CreateWorkspace)

	// KnowledgeSets
	root.Type(&v1.KnowledgeSet{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.KnowledgeSet{}).FinalizeFunc(v1.KnowledgeSetFinalizer, knowledgeset.Cleanup)
	root.Type(&v1.KnowledgeSet{}).HandlerFunc(knowledgeset.GenerateDataDescription)
	root.Type(&v1.KnowledgeSet{}).HandlerFunc(knowledgeset.CreateWorkspace)
	root.Type(&v1.KnowledgeSet{}).HandlerFunc(knowledgeset.CheckHasContent)

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
	root.Type(&v1.OAuthApp{}).HandlerFunc(reference.CreateGlobalOAuthAppReference)

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

package controller

import (
	"github.com/obot-platform/nah/pkg/handlers"
	"github.com/obot-platform/obot/pkg/controller/generationed"
	"github.com/obot-platform/obot/pkg/controller/handlers/agents"
	"github.com/obot-platform/obot/pkg/controller/handlers/alias"
	"github.com/obot-platform/obot/pkg/controller/handlers/cleanup"
	"github.com/obot-platform/obot/pkg/controller/handlers/cronjob"
	"github.com/obot-platform/obot/pkg/controller/handlers/knowledgefile"
	"github.com/obot-platform/obot/pkg/controller/handlers/knowledgeset"
	"github.com/obot-platform/obot/pkg/controller/handlers/knowledgesource"
	"github.com/obot-platform/obot/pkg/controller/handlers/knowledgesummary"
	"github.com/obot-platform/obot/pkg/controller/handlers/oauthapp"
	"github.com/obot-platform/obot/pkg/controller/handlers/runs"
	"github.com/obot-platform/obot/pkg/controller/handlers/threads"
	"github.com/obot-platform/obot/pkg/controller/handlers/toolinfo"
	"github.com/obot-platform/obot/pkg/controller/handlers/toolreference"
	"github.com/obot-platform/obot/pkg/controller/handlers/webhook"
	"github.com/obot-platform/obot/pkg/controller/handlers/workflow"
	"github.com/obot-platform/obot/pkg/controller/handlers/workflowexecution"
	"github.com/obot-platform/obot/pkg/controller/handlers/workflowstep"
	"github.com/obot-platform/obot/pkg/controller/handlers/workspace"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func (c *Controller) setupRoutes() error {
	root := c.router

	workflowExecution := workflowexecution.New(c.services.Invoker)
	workflowStep := workflowstep.New(c.services.Invoker)
	toolRef := toolreference.New(c.services.GPTClient, c.services.ModelProviderDispatcher,
		c.services.ToolRegistryURL, c.services.SupportDocker)
	workspace := workspace.New(c.services.GPTClient, c.services.WorkspaceProviderType)
	knowledgeset := knowledgeset.New(c.services.AIHelper, c.services.Invoker)
	knowledgesource := knowledgesource.NewHandler(c.services.Invoker, c.services.GPTClient)
	knowledgefile := knowledgefile.New(c.services.Invoker, c.services.GPTClient, c.services.KnowledgeSetIngestionLimit)
	runs := runs.New(c.services.Invoker)
	webHooks := webhook.New()
	cronJobs := cronjob.New()
	oauthLogins := oauthapp.NewLogin(c.services.Invoker, c.services.ServerURL)
	knowledgesummary := knowledgesummary.NewHandler(c.services.GPTClient)
	toolInfo := toolinfo.New(c.services.GPTClient)
	threads := threads.NewHandler(c.services.GPTClient)

	// Runs
	root.Type(&v1.Run{}).HandlerFunc(removeOldFinalizers)
	root.Type(&v1.Run{}).FinalizeFunc(v1.RunFinalizer, runs.DeleteRunState)
	root.Type(&v1.Run{}).HandlerFunc(runs.DeleteFinished)
	root.Type(&v1.Run{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.Run{}).HandlerFunc(runs.Resume)

	// Threads
	root.Type(&v1.Thread{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.Thread{}).HandlerFunc(threads.CreateWorkspaces)
	root.Type(&v1.Thread{}).HandlerFunc(threads.CreateKnowledgeSet)
	root.Type(&v1.Thread{}).HandlerFunc(threads.WorkflowState)
	root.Type(&v1.Thread{}).HandlerFunc(knowledgesummary.Summarize)
	root.Type(&v1.Thread{}).FinalizeFunc(v1.ThreadFinalizer, threads.CleanupThread)

	// KnowledgeSummary
	root.Type(&v1.KnowledgeSummary{}).HandlerFunc(cleanup.Cleanup)

	// Workflows
	root.Type(&v1.Workflow{}).HandlerFunc(workflow.EnsureIDs)
	root.Type(&v1.Workflow{}).HandlerFunc(workflow.CreateWorkspaceAndKnowledgeSet)
	root.Type(&v1.Workflow{}).HandlerFunc(workflow.BackPopulateAuthStatus)
	root.Type(&v1.Workflow{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.Workflow{}).HandlerFunc(alias.AssignAlias)
	root.Type(&v1.Workflow{}).HandlerFunc(toolInfo.SetToolInfoStatus)
	root.Type(&v1.Workflow{}).HandlerFunc(generationed.UpdateObservedGeneration)

	// WorkflowExecutions
	root.Type(&v1.WorkflowExecution{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.WorkflowExecution{}).HandlerFunc(workflowExecution.Run)
	root.Type(&v1.WorkflowExecution{}).HandlerFunc(workflowExecution.ReassignThread)

	// Agents
	root.Type(&v1.Agent{}).HandlerFunc(agents.CreateWorkspaceAndKnowledgeSet)
	root.Type(&v1.Agent{}).HandlerFunc(agents.BackPopulateAuthStatus)
	root.Type(&v1.Agent{}).HandlerFunc(alias.AssignAlias)
	root.Type(&v1.Agent{}).HandlerFunc(toolInfo.SetToolInfoStatus)
	root.Type(&v1.Agent{}).HandlerFunc(generationed.UpdateObservedGeneration)

	// Uploads
	root.Type(&v1.KnowledgeSource{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.KnowledgeSource{}).IncludeFinalizing().HandlerFunc(removeOldFinalizers)
	root.Type(&v1.KnowledgeSource{}).FinalizeFunc(v1.KnowledgeSourceFinalizer, knowledgesource.Cleanup)
	root.Type(&v1.KnowledgeSource{}).HandlerFunc(knowledgesource.Reschedule)
	root.Type(&v1.KnowledgeSource{}).HandlerFunc(knowledgesource.Sync)

	// ToolReferences
	root.Type(&v1.ToolReference{}).HandlerFunc(toolRef.BackPopulateModels)
	root.Type(&v1.ToolReference{}).HandlerFunc(toolRef.Populate)
	root.Type(&v1.ToolReference{}).IncludeFinalizing().HandlerFunc(removeOldFinalizers)
	root.Type(&v1.ToolReference{}).FinalizeFunc(v1.ToolReferenceFinalizer, toolRef.CleanupModelProvider)

	// EmailReceivers
	root.Type(&v1.EmailReceiver{}).HandlerFunc(alias.AssignAlias)
	root.Type(&v1.EmailReceiver{}).HandlerFunc(generationed.UpdateObservedGeneration)

	// Models
	root.Type(&v1.Model{}).HandlerFunc(deleteOldModel)
	root.Type(&v1.Model{}).HandlerFunc(alias.AssignAlias)
	root.Type(&v1.Model{}).HandlerFunc(generationed.UpdateObservedGeneration)

	// DefaultModelAliases
	root.Type(&v1.DefaultModelAlias{}).HandlerFunc(alias.AssignAlias)
	root.Type(&v1.DefaultModelAlias{}).HandlerFunc(generationed.UpdateObservedGeneration)

	// Knowledge files
	root.Type(&v1.KnowledgeFile{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.KnowledgeFile{}).IncludeFinalizing().HandlerFunc(removeOldFinalizers)
	root.Type(&v1.KnowledgeFile{}).FinalizeFunc(v1.KnowledgeFileFinalizer, knowledgefile.Cleanup)
	root.Type(&v1.KnowledgeFile{}).HandlerFunc(knowledgefile.IngestFile)
	root.Type(&v1.KnowledgeFile{}).HandlerFunc(knowledgefile.Unapproved)

	// Workspaces
	root.Type(&v1.Workspace{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.Workspace{}).IncludeFinalizing().HandlerFunc(removeOldFinalizers)
	root.Type(&v1.Workspace{}).FinalizeFunc(v1.WorkspaceFinalizer, workspace.RemoveWorkspace)
	root.Type(&v1.Workspace{}).HandlerFunc(workspace.CreateWorkspace)

	// KnowledgeSets
	root.Type(&v1.KnowledgeSet{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.KnowledgeSet{}).IncludeFinalizing().HandlerFunc(removeOldFinalizers)
	root.Type(&v1.KnowledgeSet{}).FinalizeFunc(v1.KnowledgeSetFinalizer, knowledgeset.Cleanup)
	// Also cleanup the dataset when there is no content.
	// This will allow the user to switch the embedding model implicitly.
	root.Type(&v1.KnowledgeSet{}).HandlerFunc(knowledgeset.Cleanup)
	root.Type(&v1.KnowledgeSet{}).HandlerFunc(knowledgeset.CreateWorkspace)
	root.Type(&v1.KnowledgeSet{}).HandlerFunc(knowledgeset.CheckHasContent)
	root.Type(&v1.KnowledgeSet{}).HandlerFunc(knowledgeset.SetEmbeddingModel)

	// Webhooks
	root.Type(&v1.Webhook{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.Webhook{}).HandlerFunc(alias.AssignAlias)
	root.Type(&v1.Webhook{}).HandlerFunc(webHooks.SetSuccessRunTime)
	root.Type(&v1.Webhook{}).HandlerFunc(generationed.UpdateObservedGeneration)

	// Cronjobs
	root.Type(&v1.CronJob{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.CronJob{}).HandlerFunc(cronJobs.SetSuccessRunTime)
	root.Type(&v1.CronJob{}).HandlerFunc(cronJobs.Run)

	// OAuthApps
	root.Type(&v1.OAuthApp{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.OAuthApp{}).HandlerFunc(alias.AssignAlias)
	root.Type(&v1.OAuthApp{}).HandlerFunc(generationed.UpdateObservedGeneration)

	// OAuthAppLogins
	root.Type(&v1.OAuthAppLogin{}).HandlerFunc(cleanup.Cleanup)
	root.Type(&v1.OAuthAppLogin{}).HandlerFunc(oauthLogins.RunTool)

	// Alias
	root.Type(&v1.Alias{}).HandlerFunc(alias.UnassignAlias)

	// WorkflowSteps
	steps := root.Type(&v1.WorkflowStep{})
	steps.HandlerFunc(changeWorkflowStepOwnerGVK)
	steps.HandlerFunc(cleanup.Cleanup)
	steps.HandlerFunc(handlers.GCOrphans)

	running := steps.Middleware(workflowStep.Preconditions)
	running.HandlerFunc(workflowStep.RunInvoke)
	running.HandlerFunc(workflowStep.RunIf)
	running.HandlerFunc(workflowStep.RunWhile)
	steps.HandlerFunc(workflowStep.RunSubflow)

	// AgentAuthorizations
	root.Type(&v1.AgentAuthorization{}).HandlerFunc(cleanup.Cleanup)

	c.toolRefHandler = toolRef
	return nil
}

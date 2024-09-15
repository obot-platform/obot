package controller

import (
	"github.com/acorn-io/baaah/pkg/router"
	"github.com/gptscript-ai/otto/pkg/controller/handlers/agents"
	knowledgehandler "github.com/gptscript-ai/otto/pkg/controller/handlers/knowledge"
	"github.com/gptscript-ai/otto/pkg/controller/handlers/runs"
	"github.com/gptscript-ai/otto/pkg/controller/handlers/slugs"
	"github.com/gptscript-ai/otto/pkg/controller/handlers/threads"
	"github.com/gptscript-ai/otto/pkg/controller/handlers/uploads"
	"github.com/gptscript-ai/otto/pkg/controller/handlers/workflowexecution"
	"github.com/gptscript-ai/otto/pkg/controller/handlers/workflowstep"
	"github.com/gptscript-ai/otto/pkg/controller/handlers/workspace"
	"github.com/gptscript-ai/otto/pkg/knowledge"
	"github.com/gptscript-ai/otto/pkg/services"
	v1 "github.com/gptscript-ai/otto/pkg/storage/apis/otto.gptscript.ai/v1"
)

func routes(router *router.Router, svcs *services.Services) error {
	workflowExecution := workflowexecution.New(svcs.WorkspaceClient, svcs.Invoker)
	workflowStep := workflowstep.New(svcs.Invoker)
	ingester := knowledge.NewIngester(svcs.Invoker, svcs.SystemTools[services.SystemToolKnowledge])
	agents := agents.New(svcs.WorkspaceClient, ingester, "directory", svcs.AIHelper)
	threads := threads.New(svcs.WorkspaceClient, ingester, svcs.AIHelper)
	workspace := workspace.New(svcs.WorkspaceClient, "directory")
	knowledge := knowledgehandler.New(svcs.WorkspaceClient, ingester, "directory")
	uploads := uploads.New(svcs.Invoker, svcs.WorkspaceClient, "directory", svcs.SystemTools[services.SystemToolOneDrive])
	runs := runs.New(svcs.Invoker)

	root := router

	// Runs
	root.Type(&v1.Run{}).FinalizeFunc(v1.RunFinalizer, runs.DeleteRunState)
	root.Type(&v1.Run{}).HandlerFunc(runs.Cleanup)
	root.Type(&v1.Run{}).HandlerFunc(runs.Resume)

	// Threads
	root.Type(&v1.Thread{}).FinalizeFunc(v1.ThreadFinalizer, knowledge.RemoveWorkspace)
	root.Type(&v1.Thread{}).FinalizeFunc(v1.ThreadFinalizer, workspace.RemoveWorkspace)
	root.Type(&v1.Thread{}).HandlerFunc(threads.Cleanup)
	root.Type(&v1.Thread{}).HandlerFunc(threads.Description)
	root.Type(&v1.Thread{}).HandlerFunc(knowledge.IngestKnowledge)

	// Workflows
	root.Type(&v1.Workflow{}).FinalizeFunc(v1.WorkflowFinalizer, workspace.RemoveWorkspace)
	root.Type(&v1.Workflow{}).FinalizeFunc(v1.WorkflowFinalizer, knowledge.RemoveWorkspace)
	root.Type(&v1.Workflow{}).HandlerFunc(workspace.CreateWorkspace)
	root.Type(&v1.Workflow{}).HandlerFunc(knowledge.CreateWorkspace)

	// WorkflowExecutions
	root.Type(&v1.WorkflowExecution{}).HandlerFunc(workflowExecution.Cleanup)
	root.Type(&v1.WorkflowExecution{}).HandlerFunc(workflowExecution.Run)

	// WorkflowSteps
	steps := root.Type(&v1.WorkflowStep{})
	steps.HandlerFunc(workflowStep.SetRunning)
	steps.HandlerFunc(workflowStep.Cleanup)

	running := steps.Middleware(workflowstep.Running)
	running.HandlerFunc(workflowStep.RunInvoke)
	running.HandlerFunc(workflowStep.RunIf)
	running.HandlerFunc(workflowStep.RunWhile)
	running.HandlerFunc(workflowStep.RunSubflow)

	// Agents
	root.Type(&v1.Agent{}).FinalizeFunc(v1.AgentFinalizer, workspace.RemoveWorkspace)
	root.Type(&v1.Agent{}).FinalizeFunc(v1.AgentFinalizer, knowledge.RemoveWorkspace)
	root.Type(&v1.Agent{}).HandlerFunc(agents.Suggestion)
	root.Type(&v1.Agent{}).HandlerFunc(workspace.CreateWorkspace)
	root.Type(&v1.Agent{}).HandlerFunc(knowledge.CreateWorkspace)
	root.Type(&v1.Agent{}).HandlerFunc(knowledge.IngestKnowledge)

	// Uploads
	root.Type(&v1.OneDriveLinks{}).HandlerFunc(uploads.CreateThread)
	root.Type(&v1.OneDriveLinks{}).HandlerFunc(uploads.RunUpload)
	root.Type(&v1.OneDriveLinks{}).HandlerFunc(uploads.HandleUploadRun)
	root.Type(&v1.OneDriveLinks{}).HandlerFunc(uploads.GC)
	root.Type(&v1.OneDriveLinks{}).FinalizeFunc(v1.OneDriveLinksFinalizer, uploads.Cleanup)

	// Slugs
	root.Type(&v1.Slug{}).HandlerFunc(slugs.SlugGC)
	root.Type(&v1.Agent{}).HandlerFunc(slugs.AssociateWithSlug)
	root.Type(&v1.Workflow{}).HandlerFunc(slugs.AssociateWithSlug)

	return nil
}

package render

import (
	"context"
	"fmt"
	"sort"

	"github.com/gptscript-ai/go-gptscript"
	v1 "github.com/gptscript-ai/otto/pkg/storage/apis/otto.gptscript.ai/v1"
	"github.com/gptscript-ai/otto/pkg/workspace"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var DefaultAgentParams = []string{
	"message", "Message to send",
}

type AgentOptions struct {
	Thread        *v1.Thread
	KnowledgeTool string
}

func agentKnowledgeEnv(agent *v1.Agent, thread *v1.Thread) (envs []string) {
	if agent.GetKnowledgeWorkspaceStatus().HasKnowledge {
		envs = append(envs,
			fmt.Sprintf("GPTSCRIPT_SCRIPT_ID=%s", workspace.KnowledgeIDFromWorkspaceID(agent.GetKnowledgeWorkspaceStatus().KnowledgeWorkspaceID)),
		)
		if thread != nil && thread.GetKnowledgeWorkspaceStatus().HasKnowledge {
			envs = append(envs,
				fmt.Sprintf("GPTSCRIPT_THREAD_ID=%s", workspace.KnowledgeIDFromWorkspaceID(thread.GetKnowledgeWorkspaceStatus().KnowledgeWorkspaceID)),
			)
		}
	}
	return envs
}

func Agent(ctx context.Context, db kclient.Client, agent *v1.Agent, opts AgentOptions) (_ []gptscript.ToolDef, extraEnv []string, _ error) {
	mainTool := gptscript.ToolDef{
		Name:         agent.Spec.Manifest.Name,
		Description:  agent.Spec.Manifest.Description,
		Chat:         true,
		Tools:        agent.Spec.Manifest.Tools,
		Instructions: agent.Spec.Manifest.Prompt.Instructions(),
		MetaData:     agent.Spec.Manifest.Prompt.Metadata(agent.Spec.Manifest.CodeDependencies),
		Temperature:  agent.Spec.Manifest.Temperature,
		Cache:        agent.Spec.Manifest.Cache,
		Type:         "agent",
	}
	var otherTools []gptscript.ToolDef

	if envs := agentKnowledgeEnv(agent, opts.Thread); len(envs) > 0 {
		extraEnv = envs
		if opts.KnowledgeTool != "" {
			mainTool.Tools = append(mainTool.Tools, opts.KnowledgeTool)
		}
	}

	if len(agent.Spec.Manifest.Agents) == 0 && len(agent.Spec.Manifest.Workflows) == 0 {
		return []gptscript.ToolDef{mainTool}, extraEnv, nil
	}

	agents, err := agentsByName(ctx, db, agent.Namespace)
	if err != nil {
		return nil, nil, err
	}

	for _, agentRef := range agent.Spec.Manifest.Agents {
		agent, ok := agents[agentRef]
		if !ok {
			continue
		}
		agentTool := manifestToTool(agent.Spec.Manifest, "agent", agentRef, agent.Name)
		mainTool.Tools = append(mainTool.Tools, agentTool.Name+" as "+agentRef)
		otherTools = append(otherTools, agentTool)
	}

	wfs, err := WorkflowByName(ctx, db, agent.Namespace)
	if err != nil {
		return nil, nil, err
	}

	for _, wfRef := range agent.Spec.Manifest.Workflows {
		wf, ok := wfs[wfRef]
		if !ok {
			continue
		}
		wfTool := manifestToTool(wf.Spec.Manifest.AgentManifest, "workflow", wfRef, wf.Name)
		mainTool.Tools = append(mainTool.Tools, wfTool.Name+" as "+wfRef)
		otherTools = append(otherTools, wfTool)
	}

	return append([]gptscript.ToolDef{mainTool}, otherTools...), extraEnv, nil
}

func manifestToTool(manifest v1.AgentManifest, agentType, ref, id string) gptscript.ToolDef {
	toolDef := gptscript.ToolDef{
		Name:        manifest.Name,
		Description: agentType + " described as: " + manifest.Description,
		Arguments:   manifest.GetParams(),
		Chat:        true,
	}
	if toolDef.Name == "" {
		toolDef.Name = ref
	}
	if manifest.Description == "" {
		toolDef.Description = fmt.Sprintf("Invokes %s named %s", agentType, ref)
	}
	if agentType == "agent" {
		if len(manifest.Params) == 0 {
			toolDef.Arguments = gptscript.ObjectSchema(DefaultAgentParams...)
		}
	}
	toolDef.Instructions = fmt.Sprintf(`#!/bin/bash
INPUT=$(${GPTSCRIPT_BIN} getenv GPTSCRIPT_INPUT)
if echo "${INPUT}" | grep -q '^{'; then
	echo '{"%s":"%s","type":"OttoSubFlow",'
	echo '"input":'"${INPUT}"
	echo '}'
else
	${GPTSCRIPT_BIN} sys.chat.finish "${INPUT}"
fi
`, agentType, id)
	return toolDef
}

func agentsByName(ctx context.Context, db kclient.Client, namespace string) (map[string]v1.Agent, error) {
	var agents v1.AgentList
	err := db.List(ctx, &agents, &kclient.ListOptions{
		Namespace: namespace,
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(agents.Items, func(i, j int) bool {
		return agents.Items[i].Name < agents.Items[i].Name
	})

	result := map[string]v1.Agent{}
	for _, agent := range agents.Items {
		result[agent.Name] = agent
	}

	for _, agent := range agents.Items {
		if agent.Spec.Manifest.Slug != "" {
			result[agent.Spec.Manifest.Slug] = agent
		}
	}

	for _, agent := range agents.Items {
		if agent.Spec.Manifest.Name != "" {
			result[agent.Spec.Manifest.Name] = agent
		}
	}

	return result, nil
}

func WorkflowByName(ctx context.Context, db kclient.Client, namespace string) (map[string]v1.Workflow, error) {
	var workflows v1.WorkflowList
	err := db.List(ctx, &workflows, &kclient.ListOptions{
		Namespace: namespace,
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(workflows.Items, func(i, j int) bool {
		return workflows.Items[i].Name < workflows.Items[i].Name
	})

	result := map[string]v1.Workflow{}
	for _, workflow := range workflows.Items {
		result[workflow.Name] = workflow
	}

	for _, workflow := range workflows.Items {
		if workflow.Spec.Manifest.Slug != "" {
			result[workflow.Spec.Manifest.Slug] = workflow
		}
	}

	for _, workflow := range workflows.Items {
		if workflow.Spec.Manifest.Name != "" {
			result[workflow.Spec.Manifest.Name] = workflow
		}
	}

	return result, nil
}

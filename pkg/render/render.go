package render

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/otto8-ai/otto8/apiclient/types"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.gptscript.ai/v1"
	"github.com/otto8-ai/otto8/pkg/system"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var DefaultAgentParams = []string{
	"message", "Message to send",
}

var OAuthServerURL = "http://localhost:8080"

type AgentOptions struct {
	Thread *v1.Thread
}

func Agent(ctx context.Context, db kclient.Client, agent *v1.Agent, opts AgentOptions) (_ []gptscript.ToolDef, extraEnv []string, _ error) {
	mainTool := gptscript.ToolDef{
		Name:         agent.Spec.Manifest.Name,
		Description:  agent.Spec.Manifest.Description,
		Chat:         true,
		Tools:        agent.Spec.Manifest.Tools,
		Instructions: agent.Spec.Manifest.Prompt,
		InputFilters: agent.Spec.InputFilters,
		Temperature:  agent.Spec.Manifest.Temperature,
		Cache:        agent.Spec.Manifest.Cache,
		Type:         "agent",
	}

	if mainTool.Instructions == "" {
		mainTool.Instructions = v1.DefaultAgentPrompt
	}
	var otherTools []gptscript.ToolDef

	if opts.Thread != nil {
		mainTool.Tools = append(mainTool.Tools, opts.Thread.Spec.Manifest.Tools...)
	}

	for i, tool := range agent.Spec.Manifest.Tools {
		name, err := ResolveToolReference(ctx, db, types.ToolReferenceTypeTool, agent.Namespace, tool)
		if err != nil {
			return nil, nil, err
		}
		agent.Spec.Manifest.Tools[i] = name
	}

	mainTool, otherTools, err := addAgentTools(ctx, db, agent, mainTool, otherTools)
	if err != nil {
		return nil, nil, err
	}

	mainTool, otherTools, err = addWorkflowTools(ctx, db, agent, mainTool, otherTools)
	if err != nil {
		return nil, nil, err
	}

	mainTool, otherTools, err = addKnowledgeTools(ctx, db, agent, opts.Thread, mainTool, otherTools)
	if err != nil {
		return nil, nil, err
	}

	if oauthEnv, err := setupOAuthApps(ctx, db, agent); err != nil {
		return nil, nil, err
	} else {
		extraEnv = append(extraEnv, oauthEnv...)
	}

	return append([]gptscript.ToolDef{mainTool}, otherTools...), extraEnv, nil
}

func setupOAuthApps(ctx context.Context, db kclient.Client, agent *v1.Agent) (extraEnv []string, _ error) {
	if len(agent.Spec.Manifest.OAuthApps) == 0 {
		return nil, nil
	}

	apps, err := oauthAppsByName(ctx, db, agent.Namespace)
	if err != nil {
		return nil, err
	}

	for _, appRef := range agent.Spec.Manifest.OAuthApps {
		app, ok := apps[appRef]
		if !ok {
			return nil, fmt.Errorf("oauth app %s not found", appRef)
		}
		if app.Spec.Manifest.Integration == "" {
			return nil, fmt.Errorf("oauth app %s has no integration name", app.Name)
		}

		if !app.Status.External.RefNameAssigned {
			return nil, fmt.Errorf("oauth app %s has no ref name assigned", app.Name)
		}

		integrationEnv := strings.ReplaceAll(strings.ToUpper(app.Spec.Manifest.Integration), "-", "_")

		extraEnv = append(extraEnv,
			fmt.Sprintf("GPTSCRIPT_OAUTH_%s_AUTH_URL=%s", integrationEnv, app.AuthorizeURL(OAuthServerURL)),
			fmt.Sprintf("GPTSCRIPT_OAUTH_%s_REFRESH_URL=%s", integrationEnv, app.RefreshURL(OAuthServerURL)),
			fmt.Sprintf("GPTSCRIPT_OAUTH_%s_TOKEN_URL=%s", integrationEnv, v1.OAuthAppGetTokenURL(OAuthServerURL)))
	}

	return extraEnv, nil
}

func addKnowledgeTools(ctx context.Context, db kclient.Client, agent *v1.Agent, thread *v1.Thread, mainTool gptscript.ToolDef, otherTools []gptscript.ToolDef) (_ gptscript.ToolDef, _ []gptscript.ToolDef, _ error) {
	var knowledgeSetNames []string
	knowledgeSetNames = append(knowledgeSetNames, agent.Status.KnowledgeSetNames...)
	if thread != nil {
		knowledgeSetNames = append(knowledgeSetNames, thread.Status.KnowledgeSetNames...)
	}

	if len(knowledgeSetNames) == 0 {
		return mainTool, otherTools, nil
	}

	knowledgeTool, err := ResolveToolReference(ctx, db, types.ToolReferenceTypeSystem, agent.Namespace, system.KnowledgeRetrievalTool)
	if err != nil {
		return mainTool, nil, err
	}

	for i, knowledgeSetName := range knowledgeSetNames {
		var ks v1.KnowledgeSet
		if err := db.Get(ctx, kclient.ObjectKey{Namespace: agent.Namespace, Name: knowledgeSetName}, &ks); apierror.IsNotFound(err) {
			continue
		} else if err != nil {
			return mainTool, nil, err
		}

		dataDescription := ks.Spec.Manifest.DataDescription
		if dataDescription == "" {
			dataDescription = ks.Status.SuggestedDataDescription
		}

		if dataDescription == "" {
			continue
		}

		toolName := "knowledge_set_query"
		if i > 0 {
			toolName = fmt.Sprintf("knowledge_set_%d_query", i)
		}

		tool := gptscript.ToolDef{
			Name:         toolName,
			Description:  fmt.Sprintf("Obtain search result from the knowledge set known as %s", ks.Name),
			Instructions: "#!sys.echo",
			Arguments: gptscript.ObjectSchema(
				"query", "A search query that will be evaluated against the knowledge set"),
			OutputFilters: []string{knowledgeTool + fmt.Sprintf(fmt.Sprintf(" with %s as datasets and ${query} as query", ks.Name))},
		}

		contentTool := gptscript.ToolDef{
			Name: toolName + "_context",
			Instructions: strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(`
#!sys.echo
# START INSTRUCTIONS: KNOWLEDGE SET: %n%

Use the tool %k% to query knowledge set %n% if the described data would assist in your current task.
The knowledge set %n% contains data described as:

%d%

# END INSTRUCTIONS: KNOWLEDGE SET: %n%
`, "%k%", toolName), "%d%", dataDescription), "%n%", ks.Name),
		}

		mainTool.Tools = append(mainTool.Tools, tool.Name)
		mainTool.Context = append(mainTool.Context, contentTool.Name)
		otherTools = append(otherTools, tool, contentTool)
	}

	return mainTool, otherTools, nil
}

func addWorkflowTools(ctx context.Context, db kclient.Client, agent *v1.Agent, mainTool gptscript.ToolDef, otherTools []gptscript.ToolDef) (_ gptscript.ToolDef, _ []gptscript.ToolDef, _ error) {
	if len(agent.Spec.Manifest.Workflows) == 0 {
		return mainTool, otherTools, nil
	}

	wfs, err := WorkflowByName(ctx, db, agent.Namespace)
	if err != nil {
		return mainTool, nil, err
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

	return mainTool, otherTools, nil
}

func addAgentTools(ctx context.Context, db kclient.Client, agent *v1.Agent, mainTool gptscript.ToolDef, otherTools []gptscript.ToolDef) (_ gptscript.ToolDef, _ []gptscript.ToolDef, _ error) {
	if len(agent.Spec.Manifest.Agents) == 0 {
		return mainTool, otherTools, nil
	}

	agents, err := agentsByName(ctx, db, agent.Namespace)
	if err != nil {
		return mainTool, otherTools, err
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

	return mainTool, otherTools, nil
}

func manifestToTool(manifest types.AgentManifest, agentType, ref, id string) gptscript.ToolDef {
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
#OTTO_SUBCALL: TARGET: %s
INPUT=$(${GPTSCRIPT_BIN} getenv GPTSCRIPT_INPUT)
if echo "${INPUT}" | grep -q '^{'; then
	echo '{"%s":"%s","type":"OttoSubFlow",'
	echo '"input":'"${INPUT}"
	echo '}'
else
	${GPTSCRIPT_BIN} sys.chat.finish "${INPUT}"
fi
`, id, agentType, id)
	return toolDef
}

func oauthAppsByName(ctx context.Context, c kclient.Client, namespace string) (map[string]v1.OAuthApp, error) {
	var apps v1.OAuthAppList
	err := c.List(ctx, &apps, &kclient.ListOptions{
		Namespace: namespace,
	})
	if err != nil {
		return nil, err
	}

	result := map[string]v1.OAuthApp{}
	for _, app := range apps.Items {
		result[app.Name] = app
	}

	for _, app := range apps.Items {
		if app.Spec.Manifest.RefName != "" {
			result[app.Spec.Manifest.RefName] = app
		}
	}

	return result, nil
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
		if agent.Spec.Manifest.RefName != "" && agent.Status.External.RefNameAssigned {
			result[agent.Spec.Manifest.RefName] = agent
		}
	}

	for _, agent := range agents.Items {
		if _, exists := result[agent.Spec.Manifest.Name]; !exists && agent.Spec.Manifest.Name != "" {
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
		if workflow.Spec.Manifest.RefName != "" && workflow.Status.External.RefNameAssigned {
			result[workflow.Spec.Manifest.RefName] = workflow
		}
	}

	for _, workflow := range workflows.Items {
		if _, exists := result[workflow.Spec.Manifest.Name]; !exists && workflow.Spec.Manifest.Name != "" {
			result[workflow.Spec.Manifest.Name] = workflow
		}
	}

	return result, nil
}

package render

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gz"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const knowledgeToolName = "knowledge"

var DefaultAgentParams = []string{
	"message", "Message to send",
}

type AgentOptions struct {
	Thread *v1.Thread
}

func Agent(ctx context.Context, db kclient.Client, agent *v1.Agent, oauthServerURL string, opts AgentOptions) (_ []gptscript.ToolDef, extraEnv []string, _ error) {
	defer func() {
		sort.Strings(extraEnv)
	}()

	mainTool := gptscript.ToolDef{
		Name:         agent.Spec.Manifest.Name,
		Description:  agent.Spec.Manifest.Description,
		Chat:         true,
		Instructions: agent.Spec.Manifest.Prompt,
		InputFilters: agent.Spec.InputFilters,
		Temperature:  agent.Spec.Manifest.Temperature,
		Cache:        agent.Spec.Manifest.Cache,
		Type:         "agent",
		ModelName:    agent.Spec.Manifest.Model,
		Credentials:  agent.Spec.Credentials,
	}

	extraEnv = append(extraEnv, agent.Spec.Env...)

	for _, env := range agent.Spec.Manifest.Env {
		if env.Name == "" {
			continue
		}
		if !validEnv.MatchString(env.Name) {
			return nil, nil, fmt.Errorf("invalid env var %s, must match %s", env.Name, validEnv.String())
		}
		if env.Value == "" {
			mainTool.Credentials = append(mainTool.Credentials,
				fmt.Sprintf(`github.com/gptscript-ai/credential as %s with "%s" as message and "%s" as env and %s as field`,
					env.Name, env.Description, env.Name, env.Name))
		} else {
			extraEnv = append(extraEnv, fmt.Sprintf("%s=%s", env.Name, env.Value))
		}
	}

	if mainTool.Instructions == "" {
		mainTool.Instructions = v1.DefaultAgentPrompt
	}
	var otherTools []gptscript.ToolDef

	extraEnv, added, err := configureKnowledgeEnvs(ctx, db, agent, opts.Thread, extraEnv)
	if err != nil {
		return nil, nil, err
	}

	if opts.Thread != nil {
		for _, tool := range opts.Thread.Spec.Manifest.Tools {
			if !added && tool == knowledgeToolName {
				continue
			}
			name, tools, err := Tool(ctx, db, agent.Namespace, tool)
			if err != nil {
				return nil, nil, err
			}
			if name != "" {
				mainTool.Tools = append(mainTool.Tools, name)
			}
			otherTools = append(otherTools, tools...)
		}

		credTool, err := ResolveToolReference(ctx, db, types.ToolReferenceTypeSystem, opts.Thread.Namespace, system.ExistingCredTool)
		if err != nil {
			return nil, nil, err
		}

		mainTool.Credentials = append(mainTool.Credentials, credTool+" as "+opts.Thread.Name)
		if len(opts.Thread.Spec.Env) > 0 {
			extraEnv = append(extraEnv, fmt.Sprintf("OBOT_THREAD_ENVS=%s", strings.Join(opts.Thread.Spec.Env, ",")))
		}
	}

	for _, tool := range agent.Spec.Manifest.Tools {
		if !added && tool == knowledgeToolName {
			continue
		}
		name, tools, err := Tool(ctx, db, agent.Namespace, tool)
		if err != nil {
			return nil, nil, err
		}
		if name != "" {
			mainTool.Tools = append(mainTool.Tools, name)
		}
		otherTools = append(otherTools, tools...)
	}

	for _, tool := range agent.Spec.SystemTools {
		if !added && tool == knowledgeToolName {
			continue
		}
		name, err := ResolveToolReference(ctx, db, "", agent.Namespace, tool)
		if err != nil {
			return nil, nil, err
		}
		mainTool.Tools = append(mainTool.Tools, name)
	}

	mainTool, otherTools, err = addAgentTools(ctx, db, agent, mainTool, otherTools)
	if err != nil {
		return nil, nil, err
	}

	mainTool, otherTools, err = addWorkflowTools(ctx, db, agent, mainTool, otherTools)
	if err != nil {
		return nil, nil, err
	}

	oauthEnv, err := OAuthAppEnv(ctx, db, agent.Spec.Manifest.OAuthApps, agent.Namespace, oauthServerURL)
	if err != nil {
		return nil, nil, err
	}

	extraEnv = append(extraEnv, oauthEnv...)

	return append([]gptscript.ToolDef{mainTool}, otherTools...), extraEnv, nil
}

func OAuthAppEnv(ctx context.Context, db kclient.Client, oauthAppNames []string, namespace, serverURL string) (extraEnv []string, _ error) {
	apps, err := oauthAppsByName(ctx, db, namespace)
	if err != nil {
		return nil, err
	}

	activeIntegrations := map[string]v1.OAuthApp{}
	for _, name := range slices.Sorted(maps.Keys(apps)) {
		app := apps[name]
		if app.Spec.Manifest.Global == nil || !*app.Spec.Manifest.Global || app.Spec.Manifest.ClientID == "" || app.Spec.Manifest.ClientSecret == "" || app.Spec.Manifest.Integration == "" {
			continue
		}
		activeIntegrations[app.Spec.Manifest.Integration] = app
	}

	for _, appRef := range oauthAppNames {
		app, ok := apps[appRef]
		if !ok {
			return nil, fmt.Errorf("oauth app %s not found", appRef)
		}
		if app.Spec.Manifest.Integration == "" {
			return nil, fmt.Errorf("oauth app %s has no integration name", app.Name)
		}
		if app.Spec.Manifest.ClientID == "" || app.Spec.Manifest.ClientSecret == "" {
			return nil, fmt.Errorf("oauth app %s has no client id or secret", app.Name)
		}

		activeIntegrations[app.Spec.Manifest.Integration] = app
	}

	for _, integration := range slices.Sorted(maps.Keys(activeIntegrations)) {
		app := activeIntegrations[integration]
		integrationEnv := strings.ReplaceAll(strings.ToUpper(app.Spec.Manifest.Integration), "-", "_")

		extraEnv = append(extraEnv,
			fmt.Sprintf("GPTSCRIPT_OAUTH_%s_AUTH_URL=%s", integrationEnv, app.AuthorizeURL(serverURL)),
			fmt.Sprintf("GPTSCRIPT_OAUTH_%s_REFRESH_URL=%s", integrationEnv, app.RefreshURL(serverURL)),
			fmt.Sprintf("GPTSCRIPT_OAUTH_%s_TOKEN_URL=%s", integrationEnv, v1.OAuthAppGetTokenURL(serverURL)))
	}

	return extraEnv, nil
}

// configureKnowledgeEnvs configures environment variables based on knowledge sets associated with an agent and an optional thread.
func configureKnowledgeEnvs(ctx context.Context, db kclient.Client, agent *v1.Agent, thread *v1.Thread, extraEnv []string) ([]string, bool, error) {
	var knowledgeSetNames []string
	knowledgeSetNames = append(knowledgeSetNames, agent.Status.KnowledgeSetNames...)
	if thread != nil {
		knowledgeSetNames = append(knowledgeSetNames, thread.Status.KnowledgeSetNames...)
	}

	if len(knowledgeSetNames) == 0 {
		return extraEnv, false, nil
	}

	if thread != nil {
		var knowledgeSummary v1.KnowledgeSummary
		if err := db.Get(ctx, kclient.ObjectKeyFromObject(thread), &knowledgeSummary); kclient.IgnoreNotFound(err) != nil {
			return nil, false, err
		} else if err == nil && len(knowledgeSummary.Spec.Summary) > 0 {
			var content string
			if err := gz.Decompress(&content, knowledgeSummary.Spec.Summary); err != nil {
				return nil, false, err
			}
			extraEnv = append(extraEnv, fmt.Sprintf("KNOWLEDGE_SUMMARY=%s", content))
		}
	}

	var knowledgeDatasets []string
	var knowledgeDataDescriptions []string
	for _, knowledgeSetName := range knowledgeSetNames {
		var ks v1.KnowledgeSet
		if err := db.Get(ctx, kclient.ObjectKey{Namespace: agent.Namespace, Name: knowledgeSetName}, &ks); apierror.IsNotFound(err) {
			continue
		} else if err != nil {
			return nil, false, err
		}

		if !ks.Status.HasContent {
			continue
		}

		dataDescription := agent.Spec.Manifest.KnowledgeDescription
		if dataDescription == "" {
			dataDescription = ks.Spec.Manifest.DataDescription
		}
		if dataDescription == "" {
			dataDescription = ks.Status.SuggestedDataDescription
		}

		if dataDescription == "" {
			dataDescription = "No data description available"
		}

		knowledgeDatasets = append(knowledgeDatasets, fmt.Sprintf("%s/%s", ks.Namespace, ks.Name))
		knowledgeDataDescriptions = append(knowledgeDataDescriptions, dataDescription)
	}
	if len(knowledgeDatasets) > 0 {
		extraEnv = append(extraEnv, fmt.Sprintf("KNOW_DATASETS=%s", strings.Join(knowledgeDatasets, ",")))
		extraEnv = append(extraEnv, fmt.Sprintf("KNOW_DATA_DESCRIPTIONS=%s", strings.Join(knowledgeDataDescriptions, ",")))
		return extraEnv, true, nil
	}

	return extraEnv, false, nil
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
#OBOT_SUBCALL: TARGET: %s
INPUT=$(${GPTSCRIPT_BIN} getenv GPTSCRIPT_INPUT)
if echo "${INPUT}" | grep -q '^{'; then
	echo '{"%s":"%s","type":"ObotSubFlow",'
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
		if app.Spec.Manifest.Integration != "" {
			result[app.Spec.Manifest.Integration] = app
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
		return agents.Items[i].Name < agents.Items[j].Name
	})

	result := map[string]v1.Agent{}
	for _, agent := range agents.Items {
		result[agent.Name] = agent
	}

	for _, agent := range agents.Items {
		if agent.Spec.Manifest.Alias != "" && agent.Status.AliasAssigned {
			result[agent.Spec.Manifest.Alias] = agent
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
		return workflows.Items[i].Name < workflows.Items[j].Name
	})

	result := map[string]v1.Workflow{}
	for _, workflow := range workflows.Items {
		result[workflow.Name] = workflow
	}

	for _, workflow := range workflows.Items {
		if workflow.Spec.Manifest.Alias != "" && workflow.Status.AliasAssigned {
			result[workflow.Spec.Manifest.Alias] = workflow
		}
	}

	for _, workflow := range workflows.Items {
		if _, exists := result[workflow.Spec.Manifest.Name]; !exists && workflow.Spec.Manifest.Name != "" {
			result[workflow.Spec.Manifest.Name] = workflow
		}
	}

	return result, nil
}

package render

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gz"
	"github.com/obot-platform/obot/pkg/projects"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/wait"
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

func stringAppend(first string, second ...string) string {
	if first == "" {
		return strings.Join(second, "\n\n")
	}
	if len(second) == 0 {
		return first
	}
	return strings.Join(append([]string{first}, second...), "\n\n")
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
		Credentials:  agent.Spec.Manifest.Credentials,
	}

	extraEnv = append(extraEnv, agent.Spec.Env...)

	for _, env := range agent.Spec.Manifest.Env {
		if env.Name == "" || env.Existing {
			continue
		}
		if !ValidEnv.MatchString(env.Name) {
			return nil, nil, fmt.Errorf("invalid env var %s, must match %s", env.Name, ValidEnv.String())
		}
		if env.Value == "" {
			mainTool.Credentials = append(mainTool.Credentials,
				fmt.Sprintf(`github.com/gptscript-ai/credential as %s with "%s" as message and "%s" as env and %s as field`,
					env.Name, env.Description, env.Name, env.Name))
		} else {
			extraEnv = append(extraEnv, fmt.Sprintf("%s=%s", env.Name, env.Value))
		}
	}

	if opts.Thread != nil && !opts.Thread.Status.Created {
		w, ok := db.(kclient.WithWatch)
		if ok {
			thread, err := wait.For(ctx, w, opts.Thread, func(thread *v1.Thread) (bool, error) {
				return thread.Status.Created, nil
			})
			if err != nil {
				return nil, nil, err
			}
			opts.Thread = thread
		}
	}

	if opts.Thread != nil {
		prompts, err := projects.GetStrings(ctx, db, opts.Thread, func(thread *v1.Thread) []string {
			if thread.Spec.Manifest.Prompt == "" {
				return nil
			}
			return []string{thread.Spec.Manifest.Prompt}
		})
		if err != nil {
			return nil, nil, err
		}
		mainTool.Instructions = stringAppend(mainTool.Instructions, prompts...)
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
		toolNames, err := projects.GetStrings(ctx, db, opts.Thread, func(thread *v1.Thread) []string {
			return thread.Spec.Manifest.Tools
		})
		if err != nil {
			return nil, nil, err
		}

		for _, t := range toolNames {
			if strings.HasPrefix(t, system.ToolPrefix) {
				continue
			}
			if !added && t == knowledgeToolName {
				continue
			}
			name, tools, err := tool(ctx, db, agent.Namespace, t)
			if err != nil {
				return nil, nil, err
			}
			if name != "" {
				mainTool.Tools = append(mainTool.Tools, name)
			}
			otherTools = append(otherTools, tools...)
		}

		topMost, err := projects.GetRoot(ctx, db, opts.Thread)
		if err != nil {
			return nil, nil, err
		}

		var customTools v1.ToolList
		if err := db.List(ctx, &customTools, kclient.InNamespace(topMost.Namespace), kclient.MatchingFields{
			"spec.threadName": topMost.Name,
		}); err != nil {
			return nil, nil, err
		}

		for _, customTool := range customTools.Items {
			toolDefs, err := CustomTool(ctx, db, customTool)
			if err != nil {
				return nil, nil, err
			}
			for _, toolDef := range toolDefs {
				mainTool.Tools = append(mainTool.Tools, toolDef.Name)
				otherTools = append(otherTools, toolDef)
			}
		}

		credTool, err := ResolveToolReference(ctx, db, types.ToolReferenceTypeSystem, opts.Thread.Namespace, system.ExistingCredTool)
		if err != nil {
			return nil, nil, err
		}

		mainTool.Credentials = append(mainTool.Credentials, credTool+" as "+opts.Thread.Name)

		threadWithEnv, err := projects.GetFirst(ctx, db, opts.Thread, func(parentThread *v1.Thread) (bool, error) {
			return len(parentThread.Spec.Env) > 0, nil
		})
		if err != nil {
			return nil, nil, err
		}

		var threadEnvs []string
		for _, threadEnv := range threadWithEnv.Spec.Env {
			if threadEnv.Existing && threadEnv.Name != "" {
				threadEnvs = append(threadEnvs, threadEnv.Name)
			} else if threadEnv.Value != "" {
				extraEnv = append(extraEnv, fmt.Sprintf("%s=%s", threadEnv.Name, threadEnv.Value))
			}
		}

		for _, env := range agent.Spec.Manifest.Env {
			if env.Existing && env.Name != "" {
				threadEnvs = append(threadEnvs, env.Name)
			}
		}

		if len(threadEnvs) > 0 {
			extraEnv = append(extraEnv, fmt.Sprintf("OBOT_THREAD_ENVS=%s", strings.Join(threadEnvs, ",")))
		}

		if opts.Thread.Status.SharedWorkspaceName != "" {
			var workspace v1.Workspace
			if err := db.Get(ctx, router.Key(opts.Thread.Namespace, opts.Thread.Status.SharedWorkspaceName), &workspace); err != nil {
				return nil, nil, err
			}
			extraEnv = append(extraEnv, fmt.Sprintf("DATABASE_WORKSPACE_ID=%s", workspace.Status.WorkspaceID))
		}
	}

	for _, t := range agent.Spec.Manifest.Tools {
		if !added && t == knowledgeToolName {
			continue
		}
		name, tools, err := tool(ctx, db, agent.Namespace, t)
		if err != nil {
			return nil, nil, err
		}
		if name != "" {
			mainTool.Tools = append(mainTool.Tools, name)
		}

		otherTools = append(otherTools, tools...)
	}

	mainTool, otherTools, err = addTasks(ctx, db, opts.Thread, mainTool, otherTools)
	if err != nil {
		return nil, nil, err
	}

	if opts.Thread != nil {
		for _, tool := range opts.Thread.Spec.SystemTools {
			if !added && tool == knowledgeToolName {
				continue
			}
			name, err := ResolveToolReference(ctx, db, "", agent.Namespace, tool)
			if err != nil {
				return nil, nil, err
			}
			mainTool.Tools = append(mainTool.Tools, name)
		}
	}

	extraEnv, err = setWebSiteKnowledge(ctx, db, &mainTool, agent, opts.Thread, extraEnv)
	if err != nil {
		return nil, nil, err
	}

	oauthEnv, err := OAuthAppEnv(ctx, db, agent.Spec.Manifest.OAuthApps, opts.Thread, agent.Namespace, oauthServerURL)
	if err != nil {
		return nil, nil, err
	}

	extraEnv = append(extraEnv, oauthEnv...)

	return append([]gptscript.ToolDef{mainTool}, otherTools...), extraEnv, nil
}

func mergeWebsiteKnowledge(websiteKnowledge ...*types.WebsiteKnowledge) (result types.WebsiteKnowledge) {
	for _, wk := range websiteKnowledge {
		if wk == nil {
			continue
		}
		if wk.SiteTool != "" {
			result.SiteTool = wk.SiteTool
		}
		result.Sites = append(result.Sites, wk.Sites...)
	}
	result.Sites = slices.DeleteFunc(result.Sites, func(s types.WebsiteDefinition) bool {
		return strings.TrimSpace(s.Site) == ""
	})
	return result
}

func setWebSiteKnowledge(ctx context.Context, db kclient.Client, mainTool *gptscript.ToolDef, agent *v1.Agent, thread *v1.Thread, extraEnv []string) ([]string, error) {
	threadWithWebsiteKnowledge, err := projects.GetFirst(ctx, db, thread, func(parentThread *v1.Thread) (bool, error) {
		return parentThread.Spec.Manifest.WebsiteKnowledge != nil, nil
	})
	if err != nil {
		return nil, err
	}

	var threadScoped *types.WebsiteKnowledge
	if threadWithWebsiteKnowledge != nil {
		threadScoped = threadWithWebsiteKnowledge.Spec.Manifest.WebsiteKnowledge
	}

	websiteKnowledge := mergeWebsiteKnowledge(agent.Spec.Manifest.WebsiteKnowledge, threadScoped)
	if websiteKnowledge.SiteTool == "" {
		return extraEnv, nil
	}

	if len(websiteKnowledge.Sites) == 0 {
		toRemove, _, err := tool(ctx, db, agent.Namespace, websiteKnowledge.SiteTool)
		if err != nil {
			return nil, err
		}
		mainTool.Tools = slices.DeleteFunc(mainTool.Tools, func(tool string) bool {
			return tool == toRemove
		})
		return extraEnv, nil
	}

	data, err := json.Marshal(websiteKnowledge)
	if err != nil {
		return nil, err
	}

	extraEnv = append(extraEnv, fmt.Sprintf("OBOT_WEBSITE_KNOWLEDGE=%s", string(data)))
	return extraEnv, nil
}

func OAuthAppEnv(ctx context.Context, db kclient.Client, oauthAppNames []string, thread *v1.Thread, namespace, serverURL string) (extraEnv []string, _ error) {
	projectThread, err := projects.GetRoot(ctx, db, thread)
	if err != nil {
		return nil, err
	}

	apps, err := oauthAppsByName(ctx, db, namespace, oauthAppNames, projectThread)
	if err != nil {
		return nil, err
	}

	activeIntegrations := map[string]v1.OAuthApp{}
	for _, name := range slices.Sorted(maps.Keys(apps)) {
		app := apps[name]
		if app.Spec.Manifest.Global == nil || !*app.Spec.Manifest.Global || app.Spec.Manifest.ClientID == "" || app.Spec.Manifest.Alias == "" {
			continue
		}
		activeIntegrations[app.Spec.Manifest.Alias] = app
	}

	for _, appRef := range oauthAppNames {
		app, ok := apps[appRef]
		if !ok {
			return nil, fmt.Errorf("oauth app %s not found", appRef)
		}
		if app.Spec.Manifest.Alias == "" {
			return nil, fmt.Errorf("oauth app %s has no integration name", app.Name)
		}
		if app.Spec.Manifest.ClientID == "" {
			return nil, fmt.Errorf("oauth app %s has no client id", app.Name)
		}

		activeIntegrations[app.Spec.Manifest.Alias] = app
	}

	for _, integration := range slices.Sorted(maps.Keys(activeIntegrations)) {
		app := activeIntegrations[integration]
		integrationEnv := strings.ReplaceAll(strings.ToUpper(app.Spec.Manifest.Alias), "-", "_")

		extraEnv = append(extraEnv,
			fmt.Sprintf("GPTSCRIPT_OAUTH_%s_AUTH_URL=%s", integrationEnv, app.AuthorizeURL(serverURL)),
			fmt.Sprintf("GPTSCRIPT_OAUTH_%s_REFRESH_URL=%s", integrationEnv, app.RefreshURL(serverURL)),
			fmt.Sprintf("GPTSCRIPT_OAUTH_%s_TOKEN_URL=%s", integrationEnv, app.OAuthAppGetTokenURL(serverURL)))
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

func addTasks(ctx context.Context, db kclient.Client, thread *v1.Thread, mainTool gptscript.ToolDef, otherTools []gptscript.ToolDef) (_ gptscript.ToolDef, _ []gptscript.ToolDef, _ error) {
	if thread == nil || thread.Spec.ParentThreadName == "" {
		return mainTool, otherTools, nil
	}

	var (
		wfs        v1.WorkflowList
		taskInvoke string
	)
	err := db.List(ctx, &wfs, kclient.InNamespace(thread.Namespace), kclient.MatchingFields{
		"spec.threadName": thread.Spec.ParentThreadName,
	})
	if err != nil {
		return mainTool, nil, err
	}

	added := map[string]struct{}{}
	for i, wf := range wfs.Items {
		if wf.Spec.Manifest.Name == "" {
			continue
		}
		if wf.Name == thread.Spec.WorkflowName {
			continue // skip the workflow that created this thread
		}
		if taskInvoke == "" {
			taskInvoke, err = ResolveToolReference(ctx, db, types.ToolReferenceTypeSystem, thread.Namespace, system.TaskInvoke)
			if err != nil {
				return mainTool, nil, err
			}
		}
		wfTool := manifestToTool(wf.Spec.Manifest, taskInvoke, wf.Name)
		if _, ok := added[wfTool.Name]; ok {
			wfTool.Name = fmt.Sprintf("%s %d", wfTool.Name, i+1)
		}
		mainTool.Tools = append(mainTool.Tools, wfTool.Name)
		otherTools = append(otherTools, wfTool)
		added[wfTool.Name] = struct{}{}
	}

	return mainTool, otherTools, nil
}

func manifestToTool(manifest types.WorkflowManifest, taskInvoke, id string) gptscript.ToolDef {
	taskName := strings.TrimSpace(manifest.Name)
	toolDef := gptscript.ToolDef{
		Name:        "Task " + taskName,
		Description: "Task: " + manifest.Description,
		Arguments:   types.GetParams(manifest.Params),
		Tools: []string{
			taskInvoke,
		},
		Chat: true,
	}
	if manifest.Description == "" {
		toolDef.Description = fmt.Sprintf("Invokes task named %s", taskName)
	}
	toolDef.Instructions = fmt.Sprintf(`#!sys.call %s
%s`, taskInvoke, id)
	return toolDef
}

func oauthAppsByName(ctx context.Context, c kclient.Client, namespace string, oauthNames []string, thread *v1.Thread) (map[string]v1.OAuthApp, error) {
	result := map[string]v1.OAuthApp{}

	if thread != nil {
		var apps v1.OAuthAppList
		err := c.List(ctx, &apps, kclient.InNamespace(namespace), kclient.MatchingFields{
			"spec.threadName": thread.Name,
		})
		if err != nil {
			return nil, err
		}

		for _, app := range apps.Items {
			result[app.Spec.Manifest.Alias] = app
		}
	}

	for _, oauthName := range oauthNames {
		if strings.HasPrefix(oauthName, system.OAuthAppPrefix) {
			var oauthApp v1.OAuthApp
			if err := c.Get(ctx, kclient.ObjectKey{Namespace: namespace, Name: oauthName}, &oauthApp); err != nil {
				return nil, err
			}
			result[oauthApp.Spec.Manifest.Alias] = oauthApp
		} else if _, ok := result[oauthName]; !ok {
			var apps v1.OAuthAppList
			err := c.List(ctx, &apps, kclient.InNamespace(namespace), kclient.MatchingFields{
				"spec.manifest.alias": oauthName,
			})
			if err != nil {
				return nil, err
			}
			var filtered []v1.OAuthApp
			for _, app := range apps.Items {
				if app.Spec.ThreadName == "" {
					filtered = append(filtered, app)
				}
			}
			if len(filtered) != 1 {
				return nil, fmt.Errorf("expected to find 1 OAuthApp with name %q but found %d", oauthName, len(filtered))
			}
			result[filtered[0].Spec.Manifest.Alias] = filtered[0]
		}
	}

	return result, nil
}

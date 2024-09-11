package render

import (
	"context"
	"fmt"
	"sort"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/gptscript-ai/otto/pkg/storage"
	v1 "github.com/gptscript-ai/otto/pkg/storage/apis/otto.gptscript.ai/v1"
	"github.com/gptscript-ai/otto/pkg/workspace"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var DefaultAgentParams = []string{
	"message", "Message to send to the agent",
}

func Agent(ctx context.Context, db storage.Client, agent *v1.Agent, thread *v1.Thread, knowledgeTool, knowledgeBin string) (_ []gptscript.ToolDef, extraEnv []string, _ error) {
	metadata := map[string]string{}
	if agent.Spec.Manifest.PackageJSON != "" {
		metadata["package.json"] = agent.Spec.Manifest.PackageJSON
	}
	if agent.Spec.Manifest.RequirementsTXT != "" {
		metadata["requirements.txt"] = agent.Spec.Manifest.RequirementsTXT
	}

	t := []gptscript.ToolDef{{
		Name:         agent.Spec.Manifest.Name,
		Description:  agent.Spec.Manifest.Description,
		Chat:         true,
		Tools:        agent.Spec.Manifest.Tools,
		Arguments:    agent.Spec.Manifest.GetParams(),
		Instructions: agent.Spec.Manifest.Prompt,
		MetaData:     metadata,
		Type:         "agent",
	}}

	if agent.Status.HasKnowledge || thread.Status.HasKnowledge {
		t[0].Tools = append(t[0].Tools, knowledgeTool)
		extraEnv = append(extraEnv,
			fmt.Sprintf("KNOWLEDGE_BIN=%s", knowledgeBin),
			fmt.Sprintf("GPTSCRIPT_SCRIPT_ID=%s", workspace.KnowledgeIDFromWorkspaceID(agent.Status.KnowledgeWorkspaceID)),
			fmt.Sprintf("GPTSCRIPT_THREAD_ID=%s", workspace.KnowledgeIDFromWorkspaceID(thread.Spec.KnowledgeWorkspaceID)),
		)
	}

	if len(agent.Spec.Manifest.Agents) == 0 {
		return t, extraEnv, nil
	}

	agents, err := ByName(ctx, db, agent.Namespace)
	if err != nil {
		return nil, nil, err
	}

	for _, agentRef := range agent.Spec.Manifest.Agents {
		agent, ok := agents[agentRef]
		if !ok {
			continue
		}

		toolDef := gptscript.ToolDef{
			Name:        agent.Spec.Manifest.Name,
			Description: agent.Spec.Manifest.Description,
			Arguments:   agent.Spec.Manifest.GetParams(),
		}
		if toolDef.Name == "" {
			toolDef.Name = agentRef
		}
		if toolDef.Description == "" {
			toolDef.Description = "Send message to agent named " + toolDef.Name
		}
		if len(agent.Spec.Manifest.Params) == 0 {
			toolDef.Arguments = gptscript.ObjectSchema(DefaultAgentParams...)
		}
		toolDef.Instructions = fmt.Sprintf(`#!${OTTO_BIN} invoke -t "${OTTO_THREAD_ID}.%s" "%s" "${GPTSCRIPT_INPUT}"`, agentRef, agent.Name)

		t[0].Tools = append(t[0].Tools, toolDef.Name)
		t = append(t, toolDef)
	}

	return t, extraEnv, nil
}

func ByName(ctx context.Context, db storage.Client, namespace string) (map[string]v1.Agent, error) {
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
		if _, ok := result[agent.Spec.Manifest.Slug]; !ok {
			result[agent.Spec.Manifest.Slug] = agent
		}
		if _, ok := result[agent.Spec.Manifest.Name]; !ok {
			result[agent.Spec.Manifest.Name] = agent
		}
	}

	return result, nil
}

package render

import (
	v1 "github.com/gptscript-ai/otto/pkg/storage/apis/otto.gptscript.ai/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type WorkflowOptions struct {
	Step             *v1.WorkflowStep
	ManifestOverride *v1.WorkflowManifest
}

func Workflow(wf *v1.Workflow, opts WorkflowOptions) *v1.Agent {
	agentManifest := wf.Spec.Manifest.AgentManifest
	if opts.ManifestOverride != nil {
		agentManifest = opts.ManifestOverride.AgentManifest
	}

	agent := v1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: wf.Namespace,
		},
		Spec: v1.AgentSpec{
			Manifest: agentManifest,
		},
		Status: v1.AgentStatus{
			Workspace:          wf.Status.Workspace,
			KnowledgeWorkspace: wf.Status.KnowledgeWorkspace,
		},
	}

	if step := opts.Step; step != nil {
		if step.Spec.Step.Cache != nil {
			agent.Spec.Manifest.Cache = step.Spec.Step.Cache
		}
		if step.Spec.Step.Temperature != nil {
			agent.Spec.Manifest.Temperature = step.Spec.Step.Temperature
		}
	}

	return &agent
}

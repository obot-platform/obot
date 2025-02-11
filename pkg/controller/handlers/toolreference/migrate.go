package toolreference

import (
	"context"
	"errors"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var toolMigrations = map[string]string{
	"file-summarizer-file-summarizer": "file-summarizer",
}

func MigrateToolNames(ctx context.Context, client kclient.Client) error {
	if len(toolMigrations) == 0 {
		return nil
	}

	var agents v1.AgentList
	if err := client.List(ctx, &agents); err != nil {
		return err
	}

	var workflows v1.WorkflowList
	if err := client.List(ctx, &workflows); err != nil {
		return err
	}

	var threads v1.ThreadList
	if err := client.List(ctx, &threads); err != nil {
		return err
	}

	var workflowSteps v1.WorkflowStepList
	if err := client.List(ctx, &workflowSteps); err != nil {
		return err
	}

	var objs []kclient.Object
	for _, agent := range agents.Items {
		objs = append(objs, &agent)
	}
	for _, workflow := range workflows.Items {
		objs = append(objs, &workflow)
	}
	for _, thread := range threads.Items {
		objs = append(objs, &thread)
	}
	for _, step := range workflowSteps.Items {
		objs = append(objs, &step)
	}

	var tools []string
	var errs []error
	for _, obj := range objs {
		switch o := obj.(type) {
		case *v1.Agent:
			tools = o.Spec.Manifest.Tools
		case *v1.Workflow:
			tools = o.Spec.Manifest.Tools
		case *v1.Thread:
			tools = o.Spec.Manifest.Tools
		case *v1.WorkflowStep:
			tools = o.Spec.Step.Tools
		}
		modified := false
		for i, tool := range tools {
			if newName, shouldMigrate := toolMigrations[tool]; shouldMigrate {
				tools[i] = newName
				modified = true
			}
		}

		if !modified {
			continue
		}

		errs = append(errs, client.Update(ctx, obj))
	}

	return errors.Join(errs...)
}

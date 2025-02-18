package handlers

import (
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/selectors"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type DaemonTriggerHandler struct{}

func (h *DaemonTriggerHandler) Update(req api.Context) error {
	var (
		id = req.PathValue("id")
		dt v1.DaemonTrigger
	)

	if err := req.Get(&dt, id); err != nil {
		return err
	}

	var manifest types.DaemonTriggerManifest
	if err := req.Read(&manifest); err != nil {
		return err
	}

	if err := h.validateManifest(req, manifest); err != nil {
		return err
	}

	dt.Spec.DaemonTriggerManifest = manifest
	if err := req.Update(&dt); err != nil {
		return err
	}

	return req.Write(h.convert(dt))
}

func (*DaemonTriggerHandler) Delete(req api.Context) error {
	// TODO(njhale): Make sure this works since DaemonTriggers aren't Aliasable
	return req.Delete(&v1.DaemonTrigger{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.PathValue("id"),
			Namespace: req.Namespace(),
		},
	})
}

func (h *DaemonTriggerHandler) Create(req api.Context) error {
	var manifest types.DaemonTriggerManifest
	if err := req.Read(&manifest); err != nil {
		return err
	}

	if err := h.validateManifest(req, manifest); err != nil {
		return err
	}

	dt := &v1.DaemonTrigger{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.DaemonTriggerPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.DaemonTriggerSpec{
			DaemonTriggerManifest: manifest,
		},
	}

	if err := req.Create(dt); err != nil {
		return err
	}

	return req.WriteCreated(h.convert(*dt))
}

func (h *DaemonTriggerHandler) ByID(req api.Context) error {
	var (
		dt v1.DaemonTrigger
		id = req.PathValue("id")
	)

	if err := req.Get(&dt, id); err != nil {
		return err
	}

	return req.Write(h.convert(dt))
}

func (h *DaemonTriggerHandler) List(req api.Context) error {
	var (
		daemonTriggers v1.DaemonTriggerList
		withExecutions = req.URL.Query().Get("withExecutions") == "true"
	)
	if err := req.List(&daemonTriggers, &client.ListOptions{
		FieldSelector: fields.SelectorFromSet(selectors.RemoveEmpty(map[string]string{
			"spec.provider": req.URL.Query().Get("provider"),
		})),
	}); err != nil {
		return err
	}

	var (
		resp    types.DaemonTriggerList
		visited = make(map[string][]types.WorkflowExecution, len(daemonTriggers.Items))
	)
	for _, dt := range daemonTriggers.Items {
		var executions []types.WorkflowExecution
		if withExecutions && dt.Spec.Workflow != "" {
			var ok bool
			executions, ok = visited[dt.Spec.Workflow]
			if !ok {
				// TODO(njhale): Bound this by the latest N
				var wfes v1.WorkflowExecutionList
				if err := req.List(&wfes, &client.ListOptions{
					FieldSelector: fields.SelectorFromSet(map[string]string{
						"spec.workflowName": dt.Spec.Workflow,
					}),
				}); err != nil {
					return err
				}

				for _, wfe := range wfes.Items {
					executions = append(executions, convertWorkflowExecution(wfe))
				}

				visited[dt.Spec.Workflow] = executions
			}
		}

		resp.Items = append(resp.Items, *h.convert(dt, executions...))
	}

	return req.Write(resp)
}

func (*DaemonTriggerHandler) validateManifest(req api.Context, manifest types.DaemonTriggerManifest) error {
	if manifest.Workflow == "" {
		return apierrors.NewBadRequest("webhook manifest must have a workflow name")
	}

	var workflow v1.Workflow
	if system.IsWorkflowID(manifest.Workflow) {
		if err := req.Get(&workflow, manifest.Workflow); err != nil {
			return err
		}
	}

	// TODO(njhale): Validate the daemon trigger provider exists and the options on the manifest
	if manifest.Provider == "" {
		return apierrors.NewBadRequest("daemon trigger manifest must specify a provider")
	}

	var ref v1.ToolReference
	if err := req.Get(&ref, manifest.Provider); err != nil {
		return types.NewErrBadRequest("failed to get daemon trigger provider %q: %s", manifest.Provider, err.Error())
	}
	if ref.Spec.Type != types.ToolReferenceTypeDaemonTriggerProvider {
		return types.NewErrBadRequest("%q is not a daemon trigger provider", manifest.Provider)
	}

	// TODO(njhale): Check if daemon trigger provider is configured
	// TODO(njhale): Validate configured options for daemon trigger against provider

	return nil
}

func (*DaemonTriggerHandler) convert(internal v1.DaemonTrigger, executions ...types.WorkflowExecution) *types.DaemonTrigger {
	manifest := internal.Spec.DaemonTriggerManifest
	external := &types.DaemonTrigger{
		Metadata:              MetadataFrom(&internal),
		DaemonTriggerManifest: manifest,
		WorkflowExecutions:    executions,
	}

	return external
}

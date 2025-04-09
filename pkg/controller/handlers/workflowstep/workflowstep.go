package workflowstep

import (
	"context"
	"regexp"
	"slices"
	"strings"

	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/invoke"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	invoker *invoke.Invoker
}

func New(invoker *invoke.Invoker) *Handler {
	return &Handler{
		invoker: invoker,
	}
}

func lastRunMatches(ctx context.Context, c kclient.Client, parent, current *v1.WorkflowStep) (bool, error) {
	if parent.Status.LastRunName == "" {
		if current.Status.HasRunsSet() {
			return false, nil
		}
		return true, nil
	}

	currentFirstRun := current.Status.FirstRun()
	if currentFirstRun == "" {
		return true, nil
	}

	var firstRun v1.Run
	if err := c.Get(ctx, kclient.ObjectKey{Namespace: current.Namespace, Name: currentFirstRun}, &firstRun); apierrors.IsNotFound(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return firstRun.Spec.PreviousRunName == parent.Status.LastRunName, nil
}

func deleteLastRuns(ctx context.Context, client kclient.Client, step *v1.WorkflowStep) error {
	if step.Status.LastRunName != "" {
		if err := client.Delete(ctx, &v1.Run{
			ObjectMeta: metav1.ObjectMeta{
				Name:      step.Status.LastRunName,
				Namespace: step.Namespace,
			},
		}); kclient.IgnoreNotFound(err) != nil {
			return err
		}
	}

	for _, run := range step.Status.RunNames {
		if err := client.Delete(ctx, &v1.Run{
			ObjectMeta: metav1.ObjectMeta{
				Name:      run,
				Namespace: step.Namespace,
			},
		}); kclient.IgnoreNotFound(err) != nil {
			return err
		}
	}

	step.Status.LastRunName = ""
	step.Status.RunNames = nil
	return nil
}

func (h *Handler) Preconditions(next router.Handler) router.Handler {
	return router.HandlerFunc(func(req router.Request, resp router.Response) error {
		if req.Object == nil {
			return nil
		}

		if proceed, err := h.checkPreconditions(req, resp); err != nil {
			return err
		} else if proceed {
			return next.Handle(req, resp)
		}

		return nil
	})
}

func (h *Handler) checkPreconditions(req router.Request, _ router.Response) (proceed bool, err error) {
	step := req.Object.(*v1.WorkflowStep)

	if step.Status.State.IsTerminal() {
		if !step.IsGenerationInSync() {
			// We are rerunning, reset the state and reprocess
			step.Status.State = types.WorkflowStatePending
			return false, nil
		}
		// When terminal we no longer process anything
		return false, nil
	}

	if slices.Contains(step.Status.RunNames, "") {
		step.Status.RunNames = slices.DeleteFunc(step.Status.RunNames, func(v string) bool {
			return v == ""
		})
		// Invalid state, we need to remove the empty run names
		return false, nil
	}

	// Set generation, which just means we have seen and processed this step in its current state,
	// but maybe not actually accomplished anything yet.
	step.Status.WorkflowGeneration = step.Spec.WorkflowGeneration

	if step.Spec.AfterWorkflowStepName == "" {
		// (darkness) No parents, nothing to check
		return true, nil
	}

	var parent v1.WorkflowStep
	if err := req.Get(&parent, step.Namespace, step.Spec.AfterWorkflowStepName); err != nil {
		return false, kclient.IgnoreNotFound(err)
	}

	if !parent.IsGenerationInSync() {
		// Wait for parent to be processed
		step.Status.State = types.WorkflowStatePending
		return false, nil
	}

	if parent.Status.State.IsBlocked() {
		step.Status.State = types.WorkflowStateBlocked
		return false, nil
	}

	var wf v1.WorkflowExecution
	if err := req.Get(&wf, step.Namespace, step.Spec.WorkflowExecutionName); err != nil {
		return false, err
	}

	if matchesStepID(&parent, wf.Spec.RunUntilStep) {
		// We are blocked because the workflow is supposed to only run until the parent step
		step.Status.State = types.WorkflowStateBlocked
		return false, nil
	}

	if parent.Status.State != types.WorkflowStateComplete {
		// We are just waiting for the parent to complete
		step.Status.State = types.WorkflowStatePending
		return false, nil
	}

	// If parent lastRun doesn't match our first run, we cleanup
	if matches, err := lastRunMatches(req.Ctx, req.Client, &parent, step); err != nil {
		return false, err
	} else if !matches {
		if err := deleteLastRuns(req.Ctx, req.Client, step); err != nil {
			return false, err
		}
	}

	if parent.Status.LastRunName == "" {
		step.Status.State = types.WorkflowStateBlocked
		return false, nil
	}

	if step.Status.State == "" {
		step.Status.State = types.WorkflowStatePending
	}
	return true, nil
}

func normalizeStepID(stepID string) string {
	id, _, _ := strings.Cut(stepID, "{")
	return id
}

func matchesStepID(step *v1.WorkflowStep, stepID string) bool {
	return normalizeStepID(step.Spec.Step.ID) == normalizeStepID(stepID)
}

func GetStateFromSteps[T kclient.Object](ctx context.Context, client kclient.Client, generation int64, steps ...T) (lastRun string, output string, _ types.WorkflowState, _ error) {
	var (
		toCheck []*v1.WorkflowStep
	)

	for _, obj := range steps {
		var (
			genericObj kclient.Object = obj
		)
		step := genericObj.(*v1.WorkflowStep).DeepCopy()
		if err := client.Get(ctx, kclient.ObjectKeyFromObject(step), step); apierrors.IsNotFound(err) {
			toCheck = append(toCheck, nil)
		} else if err != nil {
			return "", "", "", err
		} else if step.Status.State.IsBlocked() {
			return "", step.Status.Error, step.Status.State, nil
		}
		toCheck = append(toCheck, step)
	}

	for i, step := range toCheck {
		if step == nil || step.Status.WorkflowGeneration != generation {
			if i == 0 {
				return "", "", types.WorkflowStatePending, nil
			}
			return "", "", types.WorkflowStateRunning, nil
		}
		if i == len(steps)-1 && step.Status.State == types.WorkflowStateComplete {
			var run v1.Run
			if err := client.Get(ctx, router.Key(step.Namespace, step.Status.LastRunName), &run); err != nil {
				return "", "", "", err
			}
			return step.Status.LastRunName, run.Status.Output, types.WorkflowStateComplete, nil
		}
	}

	return "", "", types.WorkflowStateRunning, nil
}

var replaceRegexp = regexp.MustCompile(`[{},=]+`)

func NewStep(namespace, workflowExecutionName, afterStepName string, generation int64, step types.Step) *v1.WorkflowStep {
	if step.ID == "" {
		panic("step ID is required")
	}

	newID := replaceRegexp.ReplaceAllString(step.ID, "-")
	stepName := name.SafeConcatName(system.WorkflowStepPrefix+strings.TrimPrefix(workflowExecutionName, system.WorkflowExecutionPrefix), newID)
	stepName = strings.Trim(stepName, "-")
	stepName = strings.ReplaceAll(stepName, "--", "-")

	return &v1.WorkflowStep{
		ObjectMeta: metav1.ObjectMeta{
			Name:      stepName,
			Namespace: namespace,
		},
		Spec: v1.WorkflowStepSpec{
			AfterWorkflowStepName: afterStepName,
			Step:                  step,
			WorkflowExecutionName: workflowExecutionName,
			WorkflowGeneration:    generation,
		},
	}
}

package runs

import (
	"fmt"
	"time"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/nah/pkg/untriggered"
	"github.com/obot-platform/obot/pkg/invoke"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	invoker *invoke.Invoker
}

func New(invoker *invoke.Invoker) *Handler {
	return &Handler{invoker: invoker}
}

func (*Handler) DeleteRunState(req router.Request, _ router.Response) error {
	run := req.Object.(*v1.Run)
	return client.IgnoreNotFound(req.Delete(untriggered.Get(&v1.RunState{
		ObjectMeta: metav1.ObjectMeta{
			Name:      run.Name,
			Namespace: run.Namespace,
		},
	})))
}

func (h *Handler) Resume(req router.Request, _ router.Response) error {
	run := req.Object.(*v1.Run)

	if run.Status.State.IsTerminal() || run.Status.State == gptscript.Continue {
		return nil
	}

	var thread v1.Thread
	if err := req.Get(&thread, run.Namespace, run.Spec.ThreadName); apierrors.IsNotFound(err) {
		run.Status.Error = fmt.Sprintf("thread %s not found", run.Spec.ThreadName)
		run.Status.State = gptscript.Error
		return nil
	} else if err != nil {
		return err
	}

	if thread.Spec.Abort {
		run.Status.Error = "thread was aborted"
		run.Status.State = gptscript.Error
		return nil
	}

	if run.Spec.PreviousRunName != "" {
		if err := req.Get(&v1.Run{}, run.Namespace, run.Spec.PreviousRunName); apierrors.IsNotFound(err) {
			run.Status.Error = fmt.Sprintf("run %s not found: %s", run.Spec.PreviousRunName, run.Status.Error)
			run.Status.State = gptscript.Error
			return nil
		} else if err != nil {
			return err
		}
	}

	if run.Spec.Synchronous || !thread.Status.Created {
		return nil
	}

	return h.invoker.Resume(req.Ctx, req.Client, &thread, run)
}

func (h *Handler) DeleteFinished(req router.Request, _ router.Response) error {
	run := req.Object.(*v1.Run)
	if run.Status.State == gptscript.Finished && time.Since(run.Status.EndTime.Time) > 12*time.Hour || (run.Spec.Synchronous && run.Status.State == "" && time.Since(run.CreationTimestamp.Time) > 12*time.Hour) {
		// These will be system tasks. Everything is a chat and finished with Continue status
		return req.Delete(run)
	}
	return nil
}

package runs

import (
	"fmt"
	"time"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/otto8-ai/nah/pkg/router"
	"github.com/otto8-ai/otto8/logger"
	"github.com/otto8-ai/otto8/pkg/invoke"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.otto8.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var log = logger.Package()

type Handler struct {
	invoker *invoke.Invoker
}

func New(invoker *invoke.Invoker) *Handler {
	return &Handler{invoker: invoker}
}

func (*Handler) DeleteRunState(req router.Request, resp router.Response) error {
	run := req.Object.(*v1.Run)
	return client.IgnoreNotFound(req.Delete(&v1.RunState{
		ObjectMeta: metav1.ObjectMeta{
			Name:      run.Name,
			Namespace: run.Namespace,
		},
	}))
}

func (h *Handler) Resume(req router.Request, _ router.Response) error {
	run := req.Object.(*v1.Run)
	var thread v1.Thread

	if run.Status.State.IsTerminal() || run.Status.State == gptscript.Continue {
		return nil
	}

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
			run.Status.Error = fmt.Sprintf("run %s not found", run.Spec.PreviousRunName)
			run.Status.State = gptscript.Error
			return nil
		} else if err != nil {
			return err
		}
	}

	if run.Spec.Synchronous {
		if h.invoker.IsSynchronousPending(run.Name) {
			return nil
		}
		run.Status.Error = "run was interrupted most likely due to a system reset"
		run.Status.State = gptscript.Error
		return req.Client.Status().Update(req.Ctx, run)
	}

	return h.invoker.Resume(req.Ctx, req.Client, &thread, run)
}

func (h *Handler) DeleteFinished(req router.Request, _ router.Response) error {
	run := req.Object.(*v1.Run)
	if run.Status.State == gptscript.Finished && time.Since(run.Status.EndTime.Time) > 12*time.Hour {
		// These will be system tasks. Everything is a chat and finished with Continue status
		return req.Delete(run)
	}
	return nil
}

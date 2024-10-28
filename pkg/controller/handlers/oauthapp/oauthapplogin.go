package oauthapp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/acorn-io/baaah/pkg/router"
	"github.com/gptscript-ai/go-gptscript"
	"github.com/otto8-ai/otto8/pkg/invoke"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.gptscript.ai/v1"
	"github.com/otto8-ai/otto8/pkg/system"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type LoginHandler struct {
	invoker *invoke.Invoker
}

func NewLogin(invoker *invoke.Invoker) *LoginHandler {
	return &LoginHandler{
		invoker: invoker,
	}
}

func (h *LoginHandler) RunTool(req router.Request, _ router.Response) error {
	login := req.Object.(*v1.OAuthAppLogin)
	if login.Status.LoggedIn || login.Status.Error != "" || login.Spec.CredentialTool == "" {
		return nil
	}

	run, err := h.invoker.SystemAction(req.Ctx, system.ThreadPrefix+"login-", req.Namespace, invoke.SystemActionOptions{
		Events: true,
		Tools: []gptscript.ToolDef{
			{
				Credentials:  []string{login.Spec.CredentialTool},
				Instructions: "#!sys.echo DONE",
			},
		},
		CredContexts: []string{login.Spec.CredentialContext},
	})
	if err != nil {
		return err
	}

	if err = updateLoginExternalStatus(req.Ctx, req.Client, login, v1.OAuthAppLoginStatus{}); err != nil {
		return err
	}

	for frame := range run.Events {
		if frame.Prompt != nil && frame.Prompt.Metadata["authURL"] != "" {
			if err = updateLoginExternalStatus(req.Ctx, req.Client, login, v1.OAuthAppLoginStatus{
				URL: frame.Prompt.Metadata["authURL"],
			}); err != nil {
				go func() {
					// drain events
					for range run.Events {
					}
				}()
				if setErrorErr := updateLoginExternalStatus(req.Ctx, req.Client, login, v1.OAuthAppLoginStatus{
					Error: err.Error(),
				}); setErrorErr != nil {
					err = errors.Join(err, setErrorErr)
				}
				return err
			}
		}
	}

	r := run.Run
	for i := 0; i < 10; i++ {
		if err = req.Get(r, r.Namespace, r.Name); err != nil {
			continue
		}
		if r.Status.State.IsTerminal() {
			break
		}

		time.Sleep(2 * time.Second)
	}

	errMessage := r.Status.Error
	if err != nil && r.Status.Error != "" {
		errMessage = fmt.Sprintf("failed to get run: %v", err)
	}

	return updateLoginExternalStatus(req.Ctx, req.Client, login, v1.OAuthAppLoginStatus{
		Error:    errMessage,
		LoggedIn: r.Status.State == gptscript.Finished && errMessage == "",
		URL:      "",
	})
}

func updateLoginExternalStatus(ctx context.Context, client kclient.Client, login *v1.OAuthAppLogin, status v1.OAuthAppLoginStatus) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := client.Get(ctx, router.Key(login.Namespace, login.Name), login); err != nil {
			return err
		}

		login.Status = status
		return client.Status().Update(ctx, login)
	})
}

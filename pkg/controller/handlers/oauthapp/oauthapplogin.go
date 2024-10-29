package oauthapp

import (
	"context"
	"errors"

	"github.com/acorn-io/baaah/pkg/router"
	"github.com/gptscript-ai/go-gptscript"
	"github.com/otto8-ai/otto8/apiclient/types"
	"github.com/otto8-ai/otto8/pkg/invoke"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.gptscript.ai/v1"
	"github.com/otto8-ai/otto8/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	if login.Status.Authenticated || login.Status.Error != "" || login.Spec.CredentialTool == "" {
		return nil
	}

	thread := v1.Thread{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.ThreadPrefix,
			Namespace:    login.Namespace,
		},
		Spec: v1.ThreadSpec{
			OAuthAppLoginName: login.Name,
			SystemTask:        true,
		},
	}
	if err := req.Client.Create(req.Ctx, &thread); err != nil {
		return err
	}

	task, err := h.invoker.SystemTask(req.Ctx, &thread, []gptscript.ToolDef{
		{
			Credentials:  []string{login.Spec.CredentialTool},
			Instructions: "#!sys.echo DONE",
		},
	}, "", invoke.SystemTaskOptions{
		CredentialContextIDs: []string{login.Spec.CredentialContext},
	})
	if err != nil {
		return err
	}
	defer task.Close()

	if err = updateLoginExternalStatus(req.Ctx, req.Client, login, v1.OAuthAppLoginStatus{}); err != nil {
		return err
	}

	for frame := range task.Events {
		if frame.Prompt != nil && frame.Prompt.Metadata["authURL"] != "" {
			if err = updateLoginExternalStatus(req.Ctx, req.Client, login, v1.OAuthAppLoginStatus{
				OAuthAppLoginAuthStatus: types.OAuthAppLoginAuthStatus{
					URL:      frame.Prompt.Metadata["authURL"],
					Required: &[]bool{true}[0],
				},
			}); err != nil {
				if setErrorErr := updateLoginExternalStatus(req.Ctx, req.Client, login, v1.OAuthAppLoginStatus{
					OAuthAppLoginAuthStatus: types.OAuthAppLoginAuthStatus{
						Error: err.Error(),
					},
				}); setErrorErr != nil {
					err = errors.Join(err, setErrorErr)
				}
				return err
			}
		}
	}

	var errMessage string
	result, err := task.Result(req.Ctx)
	if err != nil {
		errMessage = err.Error()
	} else if result.Error != "" {
		errMessage = result.Error
	}

	return updateLoginExternalStatus(req.Ctx, req.Client, login, v1.OAuthAppLoginStatus{
		OAuthAppLoginAuthStatus: types.OAuthAppLoginAuthStatus{
			Error:         errMessage,
			Authenticated: errMessage == "",
			URL:           "",
			Required:      &[]bool{true}[0],
		},
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

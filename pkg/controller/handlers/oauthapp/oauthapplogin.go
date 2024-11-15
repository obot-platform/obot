package oauthapp

import (
	"context"
	"errors"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/otto8-ai/nah/pkg/router"
	"github.com/otto8-ai/otto8/apiclient/types"
	"github.com/otto8-ai/otto8/pkg/invoke"
	"github.com/otto8-ai/otto8/pkg/render"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.otto8.ai/v1"
	"github.com/otto8-ai/otto8/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type LoginHandler struct {
	invoker   *invoke.Invoker
	serverURL string
}

func NewLogin(invoker *invoke.Invoker, serverURL string) *LoginHandler {
	return &LoginHandler{
		invoker:   invoker,
		serverURL: serverURL,
	}
}

func (h *LoginHandler) RunTool(req router.Request, _ router.Response) error {
	login := req.Object.(*v1.OAuthAppLogin)
	if login.Status.External.Authenticated || login.Status.External.Error != "" || login.Spec.ToolReference == "" {
		return nil
	}

	credentialTool, err := v1.CredentialTool(req.Ctx, req.Client, login.Namespace, login.Spec.ToolReference)
	if err != nil || credentialTool == "" {
		return err
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

	oauthAppEnv, err := render.OAuthAppEnv(req.Ctx, req.Client, login.Spec.OAuthApps, login.Namespace, h.serverURL)
	if err != nil {
		return err
	}

	task, err := h.invoker.SystemTask(req.Ctx, &thread, []gptscript.ToolDef{
		{
			Credentials:  []string{credentialTool},
			Instructions: "#!sys.echo DONE",
		},
	}, "", invoke.SystemTaskOptions{
		CredentialContextIDs: []string{login.Spec.CredentialContext},
		Env:                  oauthAppEnv,
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
				External: types.OAuthAppLoginAuthStatus{
					URL:      frame.Prompt.Metadata["authURL"],
					Required: &[]bool{true}[0],
				},
			}); err != nil {
				if setErrorErr := updateLoginExternalStatus(req.Ctx, req.Client, login, v1.OAuthAppLoginStatus{
					External: types.OAuthAppLoginAuthStatus{
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
	_, err = task.Result(req.Ctx)
	if err != nil {
		errMessage = err.Error()
	}

	return updateLoginExternalStatus(req.Ctx, req.Client, login, v1.OAuthAppLoginStatus{
		External: types.OAuthAppLoginAuthStatus{
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

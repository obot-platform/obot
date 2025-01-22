package oauthapp

import (
	"errors"
	"time"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/invoke"
	"github.com/obot-platform/obot/pkg/render"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	credentialTools, err := v1.CredentialTools(req.Ctx, req.Client, login.Namespace, login.Spec.ToolReference)
	if err != nil || len(credentialTools) == 0 {
		return err
	}

	thread := v1.Thread{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.ThreadPrefix,
			Namespace:    login.Namespace,
			Finalizers:   []string{v1.ThreadFinalizer},
		},
		Spec: v1.ThreadSpec{
			OAuthAppLoginName: login.Name,
			SystemTask:        true,
		},
	}
	if err := req.Client.Create(req.Ctx, &thread); err != nil {
		return err
	}

	oauthAppEnv, err := render.OAuthAppEnv(req.Ctx, req.Client, login.Spec.OAuthApps, login.Namespace, h.serverURL, nil)
	if err != nil {
		return err
	}

	task, err := h.invoker.SystemTask(req.Ctx, &thread, []gptscript.ToolDef{
		{
			Credentials:  credentialTools,
			Instructions: "#!sys.echo DONE",
		},
	}, "", invoke.SystemTaskOptions{
		CredentialContextIDs: []string{login.Spec.CredentialContext},
		Env:                  oauthAppEnv,
	})
	if err != nil {
		return err
	}
	// Ensure the task is stopped when this handler returns.
	defer task.Close()

	login.Status.External = types.OAuthAppLoginAuthStatus{}
	if err = req.Client.Status().Update(req.Ctx, login); err != nil {
		return err
	}

	originalUID := login.UID
	tick := time.NewTicker(time.Second)
	defer tick.Stop()

outer:
	for {
		select {
		case <-tick.C:
			if err = req.Get(login, req.Namespace, req.Name); apierrors.IsNotFound(err) || login.UID != originalUID {
				// If the login is deleted and possibly recreated, stop blocking and retry.
				return nil
			} else if err != nil {
				return err
			}
		case frame, ok := <-task.Events:
			if !ok {
				break outer
			}

			if frame.Prompt != nil && frame.Prompt.Metadata["authURL"] != "" {
				login.Status = v1.OAuthAppLoginStatus{
					External: types.OAuthAppLoginAuthStatus{
						URL:      frame.Prompt.Metadata["authURL"],
						Required: &[]bool{true}[0],
					},
				}
				if err = req.Client.Status().Update(req.Ctx, login); err != nil {
					login.Status = v1.OAuthAppLoginStatus{
						External: types.OAuthAppLoginAuthStatus{
							Error: err.Error(),
						},
					}
					if setErrorErr := req.Client.Status().Update(req.Ctx, login); setErrorErr != nil {
						err = errors.Join(err, setErrorErr)
					}
					return err
				}
			}

			tick.Reset(time.Second)
		}
	}

	var errMessage string
	_, err = task.Result(req.Ctx)
	if err != nil {
		errMessage = err.Error()
	}

	login.Status = v1.OAuthAppLoginStatus{
		External: types.OAuthAppLoginAuthStatus{
			Error:         errMessage,
			Authenticated: errMessage == "",
			URL:           "",
			Required:      &[]bool{true}[0],
		},
	}

	return req.Client.Status().Update(req.Ctx, login)
}

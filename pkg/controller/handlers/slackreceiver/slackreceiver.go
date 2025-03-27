package slackreceiver

import (
	"fmt"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	gatewayTypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/selectors"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	gptScript *gptscript.GPTScript
}

func NewHandler(gptScript *gptscript.GPTScript) *Handler {
	return &Handler{gptScript: gptScript}
}

func (h *Handler) Reconcile(req router.Request, _ router.Response) error {
	slackReceiver := req.Object.(*v1.SlackReceiver)

	oauthAppName := system.OAuthAppPrefix + slackReceiver.Spec.ThreadName
	credential, err := h.gptScript.RevealCredential(req.Ctx, []string{oauthAppName}, oauthAppName)
	if err != nil {
		return err
	}
	clientSecret := credential.Env["CLIENT_SECRET"]
	signingSecret := credential.Env["SIGNING_SECRET"]

	if clientSecret == "" {
		return fmt.Errorf("client secret not found")
	}

	if signingSecret == "" {
		return fmt.Errorf("signing secret not found")
	}

	var oauthApps v1.OAuthAppList
	if err := req.List(&oauthApps, &kclient.ListOptions{
		Namespace: slackReceiver.Namespace,
		FieldSelector: fields.SelectorFromSet(selectors.RemoveEmpty(map[string]string{
			"spec.threadName": slackReceiver.Spec.ThreadName,
		})),
	}); err != nil {
		return err
	}

	oauthApp := v1.OAuthApp{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: slackReceiver.Namespace,
			Name:      oauthAppName,
		},
		Spec: v1.OAuthAppSpec{
			Manifest: types.OAuthAppManifest{
				ClientID: slackReceiver.Spec.Manifest.ClientID,
				Alias:    string(types.OAuthAppTypeSlack),
				Type:     types.OAuthAppTypeSlack,
			},
			ThreadName: slackReceiver.Spec.ThreadName,
		},
	}

	if err := req.Get(&oauthApp, slackReceiver.Namespace, oauthApp.Name); err != nil && apierrors.IsNotFound(err) {
		if err := gatewayTypes.ValidateAndSetDefaultsOAuthAppManifest(&oauthApp.Spec.Manifest, true); err != nil {
			return err
		}
		if err := req.Client.Create(req.Ctx, &oauthApp); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		if oauthApp.Spec.Manifest.ClientID != slackReceiver.Spec.Manifest.ClientID {
			oauthApp.Spec.Manifest.ClientID = slackReceiver.Spec.Manifest.ClientID
			if err := req.Client.Update(req.Ctx, &oauthApp); err != nil {
				return err
			}
		}
	}

	return nil
}

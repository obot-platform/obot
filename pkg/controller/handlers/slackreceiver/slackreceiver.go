package slackreceiver

import (
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	gatewayTypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CreateOAuthApp(req router.Request, _ router.Response) error {
	slackReceiver := req.Object.(*v1.SlackReceiver)

	oauthAppName := system.OAuthAppPrefix + slackReceiver.Spec.ThreadName

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
			ThreadName:        slackReceiver.Spec.ThreadName,
			SlackReceiverName: slackReceiver.Name,
		},
	}

	if err := req.Get(&oauthApp, slackReceiver.Namespace, oauthApp.Name); apierrors.IsNotFound(err) {
		if err := gatewayTypes.ValidateAndSetDefaultsOAuthAppManifest(&oauthApp.Spec.Manifest, true); err != nil {
			return err
		}
		return req.Client.Create(req.Ctx, &oauthApp)
	} else if err != nil {
		return err
	}

	if oauthApp.Spec.Manifest.ClientID != slackReceiver.Spec.Manifest.ClientID {
		oauthApp.Spec.Manifest.ClientID = slackReceiver.Spec.Manifest.ClientID
		return req.Client.Update(req.Ctx, &oauthApp)
	}

	return nil
}

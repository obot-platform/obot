package workflow

import (
	"fmt"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/otto.otto8.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apimachinery/pkg/api/equality"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func BackPopulateAuthStatus(req router.Request, _ router.Response) error {
	var updateRequired bool
	workflow := req.Object.(*v1.Workflow)

	var logins v1.OAuthAppLoginList
	if err := req.List(&logins, &kclient.ListOptions{
		FieldSelector: fields.SelectorFromSet(map[string]string{"spec.credentialContext": workflow.Name}),
		Namespace:     workflow.Namespace,
	}); err != nil {
		return err
	}

	for _, login := range logins.Items {
		if login.Status.External.Authenticated || (login.Status.External.Required != nil && !*login.Status.External.Required) || login.Spec.ToolReference == "" {
			continue
		}

		var required *bool
		credentialTool, err := v1.CredentialTool(req.Ctx, req.Client, workflow.Namespace, login.Spec.ToolReference)
		if err != nil {
			login.Status.External.Error = fmt.Sprintf("failed to get credential tool for knowledge source [%s]: %v", workflow.Name, err)
		} else {
			required = &[]bool{credentialTool != ""}[0]
			updateRequired = updateRequired || login.Status.External.Required == nil || *login.Status.External.Required != *required
			login.Status.External.Required = required
		}

		if required != nil && *required {
			var oauthAppLogin v1.OAuthAppLogin
			if err = req.Get(&oauthAppLogin, workflow.Namespace, system.OAuthAppLoginPrefix+workflow.Name+login.Spec.ToolReference); apierror.IsNotFound(err) {
				updateRequired = updateRequired || login.Status.External.Error != ""
				login.Status.External.Error = ""
			} else if err != nil {
				login.Status.External.Error = fmt.Sprintf("failed to get oauth app login for workflow [%s]: %v", workflow.Name, err)
			} else {
				updateRequired = updateRequired || equality.Semantic.DeepEqual(login.Status.External, oauthAppLogin.Status.External)
				login.Status.External = oauthAppLogin.Status.External
			}
		}

		if workflow.Status.AuthStatus == nil {
			workflow.Status.AuthStatus = make(map[string]types.OAuthAppLoginAuthStatus)
		}
		workflow.Status.AuthStatus[login.Spec.ToolReference] = login.Status.External
	}

	if updateRequired {
		return req.Client.Status().Update(req.Ctx, workflow)
	}

	return nil
}

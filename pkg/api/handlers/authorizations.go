package handlers

import (
	"github.com/gptscript-ai/gptscript/pkg/hash"
	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/client"
	types2 "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type AuthorizationHandler struct {
	userClient *client.Client
}

func AgentAuthorizationName(agentID string, userID string) string {
	return name.SafeHashConcatName(agentID, hash.Digest(userID))
}

func NewAuthorizationHandler(userClient *client.Client) *AuthorizationHandler {
	return &AuthorizationHandler{
		userClient: userClient,
	}
}

func (a *AuthorizationHandler) AddAgentAuthorization(req api.Context) error {
	var (
		agentID      = req.PathValue("id")
		authManifest types.AgentAuthorizationManifest
	)

	if err := req.Read(&authManifest); err != nil {
		return err
	}

	authManifest.AgentID = agentID

	grant := &v1.AgentAuthorization{
		ObjectMeta: metav1.ObjectMeta{
			Name:      AgentAuthorizationName(authManifest.AgentID, authManifest.UserID),
			Namespace: req.Namespace(),
		},
		Spec: v1.AgentAuthorizationSpec{
			AgentAuthorizationManifest: authManifest,
		},
	}

	if err := req.Create(grant); kclient.IgnoreAlreadyExists(err) != nil {
		return err
	}

	return req.Write(authManifest)
}

func (a *AuthorizationHandler) RemoveAgentAuthorization(req api.Context) error {
	var (
		agentID      = req.PathValue("id")
		authManifest types.AgentAuthorizationManifest
	)

	if err := req.Read(&authManifest); err != nil {
		return err
	}

	authManifest.AgentID = agentID

	err := req.Delete(&v1.AgentAuthorization{
		ObjectMeta: metav1.ObjectMeta{
			Name:      AgentAuthorizationName(agentID, authManifest.UserID),
			Namespace: req.Namespace(),
		},
	})
	return kclient.IgnoreNotFound(err)
}

func (a *AuthorizationHandler) ListAgentAuthorizations(req api.Context) error {
	var (
		agentID = req.PathValue("id")
		auths   v1.AgentAuthorizationList
	)

	err := req.List(&auths, kclient.MatchingFields{
		"spec.agentID": agentID,
	})
	if err != nil {
		return err
	}

	result := types.AuthorizationList{
		Items: make([]types.AgentAuthorization, 0, len(auths.Items)),
	}

	for _, grant := range auths.Items {
		auth := types.AgentAuthorization{
			AgentAuthorizationManifest: grant.Spec.AgentAuthorizationManifest,
		}

		// Yes, this is N+1 but will be fine for now ¯\_(ツ)_/¯
		// It's faster than having the client look up each user individually
		user, err := a.userClient.UserByID(req.Context(), grant.Spec.UserID)
		if err != nil {
			log.Errorf("failed to get user for authorization list %s: %v", grant.Spec.UserID, err)
		} else if user != nil {
			auth.User = types2.ConvertUser(user, a.userClient.IsExplicitAdmin(user.Email))
		}

		result.Items = append(result.Items, auth)
	}

	return req.Write(result)
}

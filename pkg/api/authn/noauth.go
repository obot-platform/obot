package authn

import (
	"fmt"
	"net/http"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

type NoAuth struct {
	client *client.Client
}

func NewNoAuth(client *client.Client) *NoAuth {
	return &NoAuth{
		client: client,
	}
}

func (n *NoAuth) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	gatewayUser, err := n.client.EnsureIdentityWithRole(
		req.Context(),
		&types.Identity{
			ProviderUsername: "nobody",
			ProviderUserID:   "nobody",
		},
		req.Header.Get("X-Obot-User-Timezone"),
		types2.RoleOwner|types2.RoleAuditor,
	)
	if err != nil {
		return nil, false, err
	}

	return &authenticator.Response{
		User: &user.DefaultInfo{
			Name:   "nobody",
			UID:    fmt.Sprintf("%d", gatewayUser.ID),
			Groups: gatewayUser.Role.Groups(),
		},
	}, true, nil
}

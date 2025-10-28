package server

import (
	"fmt"
	"strconv"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/types"
)

func (s *Server) listIdentitiesByUser(apiContext api.Context) error {
	userID, err := strconv.ParseUint(apiContext.PathValue("user_id"), 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse user ID: %v", err)
	}

	identities, err := apiContext.GatewayClient.FindIdentitiesForUser(apiContext.Context(), uint(userID))
	if err != nil {
		return fmt.Errorf("failed to get users: %v", err)
	}

	items := make([]types2.Identity, 0, len(identities))
	for _, id := range identities {
		if id.ProviderUsername != "bootstrap" && id.Email != "" { // Filter out the bootstrap admin
			items = append(items, types.ConvertIdentity(id))
		}
	}

	return apiContext.Write(types2.IdentityList{Items: items})
}

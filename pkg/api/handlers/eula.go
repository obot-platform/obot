package handlers

import (
	"errors"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"gorm.io/gorm"
)

const eulaAcceptedKey = "eula_accepted"

type EulaHandler struct{}

func NewEulaHandler() *EulaHandler {
	return &EulaHandler{}
}

// Get retrieves the EULA acceptance status for the installation
func (h *EulaHandler) Get(req api.Context) error {
	// EULA is global for the entire installation, not per-user
	key := eulaAcceptedKey
	property, err := req.GatewayClient.GetProperty(req.Context(), key)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// EULA has not been accepted yet
			return req.Write(types.EulaStatus{
				Accepted: false,
			})
		}
		return err
	}

	return req.Write(types.EulaStatus{
		Accepted: property.Value == "true",
	})
}

// Update records the EULA acceptance or decline for the installation
func (h *EulaHandler) Update(req api.Context) error {
	var input types.EulaStatus
	if err := req.Read(&input); err != nil {
		return err
	}

	// EULA is global for the entire installation, not per-user
	key := eulaAcceptedKey
	value := "false"
	if input.Accepted {
		value = "true"
	}

	property, err := req.GatewayClient.SetProperty(req.Context(), key, value)
	if err != nil {
		return err
	}

	return req.Write(types.EulaStatus{
		Accepted: property.Value == "true",
	})
}

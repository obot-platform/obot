package handlers

import (
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type UserDefaultRoleSettingHandler struct {
	storageClient storage.Client
}

func NewUserDefaultRoleSettingHandler(storageClient storage.Client) *UserDefaultRoleSettingHandler {
	return &UserDefaultRoleSettingHandler{
		storageClient: storageClient,
	}
}

func (h *UserDefaultRoleSettingHandler) Get(req api.Context) error {
	var setting v1.UserDefaultRoleSetting
	if err := h.storageClient.Get(req.Context(), client.ObjectKey{Namespace: req.Namespace(), Name: system.DefaultRoleSettingName}, &setting); err != nil {
		return err
	}
	return req.Write(convertUserDefaultRoleSetting(setting))
}

func (h *UserDefaultRoleSettingHandler) Set(req api.Context) error {
	var input types.UserDefaultRoleSetting
	if err := req.Read(&input); err != nil {
		return err
	}

	var setting v1.UserDefaultRoleSetting
	if err := h.storageClient.Get(req.Context(), client.ObjectKey{Namespace: req.Namespace(), Name: system.DefaultRoleSettingName}, &setting); err != nil {
		return err
	}

	setting.Spec.Role = input.Role

	if err := h.storageClient.Update(req.Context(), &setting); err != nil {
		return err
	}
	return req.Write(convertUserDefaultRoleSetting(setting))
}

func convertUserDefaultRoleSetting(setting v1.UserDefaultRoleSetting) types.UserDefaultRoleSetting {
	return types.UserDefaultRoleSetting{
		Role: setting.Spec.Role,
	}
}

package handlers

import (
	"errors"
	"net/http"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type UserDefaultRoleSettingHandler struct{}

func NewUserDefaultRoleSettingHandler() *UserDefaultRoleSettingHandler {
	return nil
}

func (*UserDefaultRoleSettingHandler) Get(req api.Context) error {
	var setting v1.UserDefaultRoleSetting
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{Namespace: req.Namespace(), Name: system.DefaultRoleSettingName}, &setting); err != nil {
		return err
	}
	return req.Write(convertUserDefaultRoleSetting(setting))
}

func (*UserDefaultRoleSettingHandler) Set(req api.Context) error {
	var input types.UserDefaultRoleSetting
	if err := req.Read(&input); err != nil {
		if _, ok := errors.AsType[*types.ErrHTTP](err); ok {
			return err
		}
		return types.NewErrHTTP(http.StatusBadRequest, "invalid default role request body")
	}
	if !input.Role.IsValidUserRole() {
		return types.NewErrHTTP(http.StatusBadRequest, "invalid default role combination")
	}
	if (input.Role.ExtractBaseRole() == types.RoleOwner || input.Role.HasAuditorRole()) && !req.UserIsOwner() {
		return types.NewErrHTTP(http.StatusForbidden, "only owners can set a default role that includes Owner or Auditor")
	}

	var setting v1.UserDefaultRoleSetting
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{Namespace: req.Namespace(), Name: system.DefaultRoleSettingName}, &setting); err != nil {
		return err
	}

	setting.Spec.Role = input.Role

	if err := req.Storage.Update(req.Context(), &setting); err != nil {
		return err
	}
	return req.Write(convertUserDefaultRoleSetting(setting))
}

func convertUserDefaultRoleSetting(setting v1.UserDefaultRoleSetting) types.UserDefaultRoleSetting {
	return types.UserDefaultRoleSetting{
		Role: setting.Spec.Role,
	}
}

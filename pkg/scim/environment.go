package scim

import (
	"context"
	"fmt"

	types2 "github.com/obot-platform/obot/apiclient/types"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ConfiguredAuthProviderGetter returns the name of the auth provider that serves sign-ins.
type ConfiguredAuthProviderGetter interface {
	GetConfiguredAuthProvider(ctx context.Context) (string, error)
}

type environment struct {
	authProviders ConfiguredAuthProviderGetter
	userLimits    gclient.UserLimitProvider
	storage       kclient.Client
}

// NewEnvironment returns the Environment of a running Obot server.
func NewEnvironment(authProviders ConfiguredAuthProviderGetter, userLimits gclient.UserLimitProvider, storage kclient.Client) Environment {
	return &environment{
		authProviders: authProviders,
		userLimits:    userLimits,
		storage:       storage,
	}
}

func (e *environment) ConfiguredAuthProvider(ctx context.Context) (string, string, error) {
	name, err := e.authProviders.GetConfiguredAuthProvider(ctx)
	if err != nil || name == "" {
		return "", "", err
	}
	// Auth providers always live in the default namespace.
	return system.DefaultNamespace, name, nil
}

func (e *environment) UserLimit(ctx context.Context) (gclient.UserLimit, error) {
	return e.userLimits.UserLimit(ctx)
}

func (e *environment) DefaultRole(ctx context.Context) (types2.Role, error) {
	var setting v1.UserDefaultRoleSetting
	if err := e.storage.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: system.DefaultRoleSettingName}, &setting); err != nil {
		return types2.RoleUnknown, fmt.Errorf("failed to get default role setting: %w", err)
	}
	return setting.Spec.Role, nil
}

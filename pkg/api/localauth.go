package api

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/obot-platform/obot/apiclient/types"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/i18n"
	"github.com/obot-platform/obot/pkg/localauth"
)

// LocalAuthCreateUserError converts a non-nil error from localauth.Provider.CreateUser into the
// error an API handler returns, localizing the ones that are the caller's fault.
func LocalAuthCreateUserError(req Context, err error) error {
	locale := req.Locale()
	if errors.Is(err, gateway.ErrBootstrapLocalAuthUserLimit) {
		return types.NewErrHTTP(http.StatusConflict, i18n.Message(locale, "local_bootstrap_limit", nil))
	}
	if errors.Is(err, gateway.ErrLocalAuthUserExists) {
		return types.NewErrBadRequest("%s", i18n.Message(locale, "local_user_exists", nil))
	}
	if invalid, ok := errors.AsType[localauth.InvalidUserError](err); ok {
		return types.NewErrBadRequest("%s", invalid.Localized(locale))
	}
	return fmt.Errorf("failed to create local auth user: %w", err)
}

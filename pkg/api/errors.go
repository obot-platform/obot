//nolint:revive
package api

import (
	"net/http"

	"github.com/obot-platform/obot/apiclient/types"
)

var ErrMustAuth = &types.ErrHTTP{
	Code:    http.StatusUnauthorized,
	Message: "unauthorized request, must authenticate",
}

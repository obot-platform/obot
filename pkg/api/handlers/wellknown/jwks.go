package wellknown

import (
	"github.com/obot-platform/obot/pkg/api"
)

func (h *handler) jwks(req api.Context) error {
	return req.Write(h.keySet)
}

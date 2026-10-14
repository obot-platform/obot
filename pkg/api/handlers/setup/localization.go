package setup

import (
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/i18n"
)

func localized(req api.Context, key string, values ...string) string {
	locale := "en"
	if req.Request != nil {
		locale = i18n.FromContext(req.Context())
	}
	args := make(map[string]string, len(values)/2)
	for i := 0; i+1 < len(values); i += 2 {
		args[values[i]] = values[i+1]
	}
	return i18n.Message(locale, key, args)
}

package setup

import (
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/i18n"
)

func localized(req api.Context, key string, values ...string) string {
	if req.ResponseWriter != nil {
		req.ResponseWriter.Header().Add("Vary", "Accept-Language")
	}
	locale := "en"
	if req.Request != nil {
		locale = i18n.Locale(req.Request.Header.Get("Accept-Language"))
	}
	args := make(map[string]string, len(values)/2)
	for i := 0; i+1 < len(values); i += 2 {
		args[values[i]] = values[i+1]
	}
	return i18n.Message(locale, key, args)
}

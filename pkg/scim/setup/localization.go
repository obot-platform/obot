package setup

import (
	"context"

	"github.com/obot-platform/obot/pkg/i18n"
)

func message(ctx context.Context, key string, values ...string) string {
	if len(values)%2 != 0 {
		panic("SCIM message arguments must be key/value pairs")
	}
	args := make(map[string]string, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		args[values[i]] = values[i+1]
	}
	return i18n.Message(i18n.FromContext(ctx), key, args)
}

//nolint:revive
package log

import "log/slog"

func NewWithID(id string) *slog.Logger {
	log := slog.Default().With("logger", "gateway")
	if id != "" {
		return log.With("req_id", id)
	}
	return log
}

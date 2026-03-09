//nolint:revive
package log

import "github.com/obot-platform/obot/logger"

func New() *logger.Logger {
	return NewWithID("")
}

func NewWithID(id string) *logger.Logger {
	log := logger.New("gateway")
	if id != "" {
		return log.Fields("req_id", id)
	}
	return &log
}

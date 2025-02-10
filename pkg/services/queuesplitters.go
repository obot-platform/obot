package services

import (
	"github.com/obot-platform/nah/pkg/runtime"
	"github.com/obot-platform/obot/pkg/system"
)

type runQueueSplitter struct{}

func (*runQueueSplitter) Split(key string) int {
	_, name := runtime.KeyParse(key)
	if system.IsChatRunID(name) {
		return 1
	}

	return 0
}

func (*runQueueSplitter) Queues() int {
	return 2
}

package handlers

import (
	"github.com/otto8-ai/otto8/pkg/api"
	"github.com/otto8-ai/otto8/pkg/version"
)

func GetVersion(req api.Context) error {
	return req.Write(version.Get().String())
}

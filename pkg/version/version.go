package version

import (
	"github.com/obot-platform/nah/pkg/version"
)

var (
	Tag = "v0.0.0-dev"
)

func Get() version.Version {
	return version.NewVersion(Tag)
}

package scheme

import (
	v1 "github.com/acorn-io/acorn/pkg/storage/apis/otto.otto8.ai/v1"
	"github.com/acorn-io/nah/pkg/restconfig"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
)

//nolint:revive
var Scheme, Codecs, Parameter, AddToScheme = restconfig.MustBuildScheme(
	v1.AddToScheme,
	coordinationv1.AddToScheme,
	corev1.AddToScheme,
)

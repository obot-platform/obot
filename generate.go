//go:generate go run github.com/acorn-io/nah/cmd/deepcopy ./pkg/storage/apis/otto.otto8.ai/v1/
//go:generate go run github.com/acorn-io/nah/cmd/deepcopy ./apiclient/types/
//go:generate go run k8s.io/kube-openapi/cmd/openapi-gen --go-header-file tools/header.txt --output-file openapi_generated.go --output-dir ./pkg/storage/openapi/generated/ --output-pkg github.com/acorn-io/acorn/pkg/storage/openapi/generated github.com/acorn-io/acorn/pkg/storage/apis/otto.otto8.ai/v1 k8s.io/apimachinery/pkg/apis/meta/v1 k8s.io/apimachinery/pkg/runtime k8s.io/apimachinery/pkg/version k8s.io/apimachinery/pkg/api/resource k8s.io/apimachinery/pkg/util/intstr k8s.io/api/coordination/v1 github.com/acorn-io/acorn/apiclient/types

package main

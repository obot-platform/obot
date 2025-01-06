//go:generate go run github.com/obot-platform/nah/cmd/deepcopy ./pkg/storage/apis/obot.obot.ai/v1/
//go:generate go run github.com/obot-platform/nah/cmd/deepcopy ./apiclient/types/
//go:generate go run k8s.io/kube-openapi/cmd/openapi-gen --go-header-file tools/header.txt --output-file openapi_generated.go --output-dir ./pkg/storage/openapi/generated/ --output-pkg github.com/obot-platform/obot/pkg/storage/openapi/generated github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1 k8s.io/apimachinery/pkg/apis/meta/v1 k8s.io/apimachinery/pkg/runtime k8s.io/apimachinery/pkg/version k8s.io/apimachinery/pkg/api/resource k8s.io/apimachinery/pkg/util/intstr k8s.io/api/coordination/v1 github.com/obot-platform/obot/apiclient/types

package main

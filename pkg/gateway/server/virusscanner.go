package server

import (
	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (s *Server) getFileScannerConfig(apiContext api.Context) error {
	config, err := s.client.GetVirusScannerConfig(apiContext.Context())
	if err != nil {
		return err
	}

	return apiContext.Write(config)
}

func (s *Server) updateFileScannerConfig(apiContext api.Context) error {
	var config types.FileScannerConfig
	if err := apiContext.Read(&config); err != nil {
		return err
	}

	if config.ProviderName != "" && config.ProviderNamespace == "" {
		// The provider namespace should be the system namespace in this case
		config.ProviderNamespace = system.DefaultNamespace
	}

	// Ensure this provider exists
	var ref v1.ToolReference
	if err := apiContext.Storage.Get(apiContext.Context(), kclient.ObjectKey{Namespace: config.ProviderNamespace, Name: config.ProviderName}, &ref); err != nil {
		return err
	} else if ref.Spec.Type != types2.ToolReferenceTypeFileScannerProvider {
		return types2.NewErrBadRequest("%q is not a file scanner provider", ref.Name)
	}

	if err := s.client.UpdateVirusScannerConfig(apiContext.Context(), &config); err != nil {
		return err
	}

	return apiContext.Write(config)
}

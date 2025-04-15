package providers

import (
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func ConvertFileScannerProviderToolRef(toolRef v1.ToolReference, cred map[string]string) (*types.FileScannerProviderStatus, error) {
	providerStatus, err := ConvertProviderTooRef(toolRef, cred)
	if err != nil {
		return nil, err
	}

	return &types.FileScannerProviderStatus{CommonProviderStatus: *providerStatus}, nil
}

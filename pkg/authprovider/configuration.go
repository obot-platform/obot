package authprovider

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

type daemonConfiguration struct {
	UID                   string              `json:"uid"`
	Spec                  v1.AuthProviderSpec `json:"spec"`
	CredentialEnvironment map[string]string   `json:"credentialEnvironment"`
}

// DaemonConfigurationHash returns the canonical hash of all persisted inputs
// that affect an auth provider daemon. Process-level environment is excluded
// because changing it requires restarting Obot itself.
func DaemonConfigurationHash(authProvider v1.AuthProvider, credentialEnvironment map[string]string) (string, error) {
	if credentialEnvironment == nil {
		credentialEnvironment = map[string]string{}
	}

	data, err := json.Marshal(daemonConfiguration{
		UID:                   string(authProvider.UID),
		Spec:                  authProvider.Spec,
		CredentialEnvironment: credentialEnvironment,
	})
	if err != nil {
		return "", fmt.Errorf("marshal auth provider daemon configuration: %w", err)
	}

	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

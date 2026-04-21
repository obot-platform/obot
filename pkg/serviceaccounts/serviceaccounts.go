package serviceaccounts

import (
	"fmt"
	"maps"
	"slices"
)

const (
	Group                   = "system:serviceaccounts"
	TokenPrefix             = "osa1"
	NetworkPolicyProvider   = "NetworkPolicyProvider"
	NetworkPolicySecretName = "obot-network-policy-provider"
	NetworkPolicySecretKey  = "apiKey"
	ServiceAccountNameKey   = "serviceAccountName"
	RotatedAtKey            = "rotatedAt"
	ExpiresAtKey            = "expiresAt"
)

type Account struct {
	Name               string
	Username           string
	UID                string
	Group              string
	SecretName         string
	SecretManaged      bool
	RequiredMCPBackend string
}

var accounts = map[string]Account{
	NetworkPolicyProvider: {
		Name:               NetworkPolicyProvider,
		Username:           "system:serviceaccount:" + NetworkPolicyProvider,
		UID:                "system:serviceaccount:" + NetworkPolicyProvider,
		Group:              fmt.Sprintf("%s:%s", Group, NetworkPolicyProvider),
		SecretName:         NetworkPolicySecretName,
		SecretManaged:      true,
		RequiredMCPBackend: "kubernetes",
	},
}

func All() []Account {
	return slices.Collect(maps.Values(accounts))
}

func Get(name string) (Account, bool) {
	account, ok := accounts[name]
	return account, ok
}

func Enabled(account Account, mcpRuntimeBackend string) bool {
	if account.RequiredMCPBackend == "" {
		return true
	}
	return account.RequiredMCPBackend == mcpRuntimeBackend
}

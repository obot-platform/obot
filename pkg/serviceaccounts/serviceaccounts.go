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
	Name                         string
	Username                     string
	UID                          string
	Group                        string
	SecretName                   string
	SecretKey                    string
	SecretManaged                bool
	RequiredMCPBackend           string
	RequiredNetworkPolicyEnabled bool
}

var accounts = map[string]Account{
	NetworkPolicyProvider: {
		Name:                         NetworkPolicyProvider,
		Username:                     "system:serviceaccount:" + NetworkPolicyProvider,
		UID:                          "system:serviceaccount:" + NetworkPolicyProvider,
		Group:                        fmt.Sprintf("%s:%s", Group, NetworkPolicyProvider),
		SecretName:                   NetworkPolicySecretName,
		SecretKey:                    NetworkPolicySecretKey,
		SecretManaged:                true,
		RequiredMCPBackend:           "kubernetes",
		RequiredNetworkPolicyEnabled: true,
	},
}

func All() []Account {
	return slices.Collect(maps.Values(accounts))
}

func Get(name string) (Account, bool) {
	account, ok := accounts[name]
	return account, ok
}

func Enabled(account Account, mcpRuntimeBackend string, networkPolicyEnabled bool) bool {
	if account.RequiredMCPBackend != "" && !mcpBackendMatches(account.RequiredMCPBackend, mcpRuntimeBackend) {
		return false
	}
	if account.RequiredNetworkPolicyEnabled && !networkPolicyEnabled {
		return false
	}
	return true
}

func mcpBackendMatches(required, actual string) bool {
	if required == actual {
		return true
	}
	// "kubernetes" matches any kubernetes variant (k8s, k8s-local)
	return required == "kubernetes" && (actual == "k8s" || actual == "k8s-local")
}

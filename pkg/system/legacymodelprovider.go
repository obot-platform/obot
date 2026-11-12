package system

// IsLegacyModelProvider distinguishes retained source configuration from running backends.
func IsLegacyModelProvider(provider string) bool {
	return provider != "" && provider != LLMGatewayModelProvider && provider != ModelProxyModelProvider
}

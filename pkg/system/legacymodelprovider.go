package system

// LegacyModelMigrationAnnotation records the destination after publication and stored
// references succeed. Retained sources must not replay settings or reclaim aliases.
const LegacyModelMigrationAnnotation = "obot.ai/llm-gateway-migrated-model"

// IsLegacyModelProvider distinguishes retained source configuration from running backends.
func IsLegacyModelProvider(provider string) bool {
	return provider != "" && provider != LLMGatewayModelProvider && provider != ModelProxyModelProvider
}

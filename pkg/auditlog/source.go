package auditlog

import api "github.com/obot-platform/obot/apiclient/types"

// NormalizeSourceTypes applies the MCP-only default and removes duplicate source types while
// preserving their original order. It intentionally preserves unknown values so callers can
// reject them during validation rather than silently changing the requested source selection.
func NormalizeSourceTypes(sourceTypes []api.AuditLogSourceType) []api.AuditLogSourceType {
	if len(sourceTypes) == 0 {
		return []api.AuditLogSourceType{api.AuditLogSourceTypeMCP}
	}

	seen := make(map[api.AuditLogSourceType]struct{}, len(sourceTypes))
	result := make([]api.AuditLogSourceType, 0, len(sourceTypes))
	for _, sourceType := range sourceTypes {
		if _, ok := seen[sourceType]; ok {
			continue
		}
		seen[sourceType] = struct{}{}
		result = append(result, sourceType)
	}
	return result
}

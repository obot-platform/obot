// Shared helper for the audit-log export forms (CreateAuditLogExportForm and CreateScheduleForm).
// Both forms render the same MCP/local-agent filter fields and hide the source-specific ones the
// same way, so the visibility logic lives here to avoid drifting between the two.

// Decide which filter fields/rows should be visible for the currently selected source(s).
//
// Source-specific filters require exactly one selected source (the backend rejects them for
// mixed-source exports). Keep a group visible if it still holds values so the user can clear a
// stale selection after switching sources, instead of both groups hiding at once.
export function filterVisibleExportFields<TField extends { filterKey: string }>(
	form: { sourceTypes: readonly string[]; filters: Record<string, unknown> },
	fields: readonly TField[],
	mcpFilterKeys: ReadonlySet<string>,
	localFilterKeys: ReadonlySet<string>
): TField[] {
	const hasMCPFilters = [...mcpFilterKeys].some((key) => form.filters[key]);
	const hasLocalFilters = [...localFilterKeys].some((key) => form.filters[key]);
	const onlyMCP = form.sourceTypes.length === 1 && form.sourceTypes.includes('mcp');
	const onlyLocal =
		form.sourceTypes.length === 1 && form.sourceTypes.includes('local_agent_tool_call');

	return fields.filter((field) => {
		if (mcpFilterKeys.has(field.filterKey)) {
			return hasMCPFilters || onlyMCP;
		}
		if (localFilterKeys.has(field.filterKey)) {
			return hasLocalFilters || onlyLocal;
		}
		return true;
	});
}

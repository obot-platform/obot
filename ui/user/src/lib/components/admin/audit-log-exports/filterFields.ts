// Shared helpers for the audit-log export forms (CreateAuditLogExportForm and CreateScheduleForm).
// Both forms offer the same log-source selection and render the same MCP/local-agent filter fields,
// so that logic lives here to avoid drifting between the two.

export const ALL_SOURCE_TYPES = ['mcp', 'local_agent_tool_call'] as const;

export const sourceTypeLabels: Record<string, string> = {
	mcp: 'MCP',
	local_agent_tool_call: 'Local Agent Tool Calls'
};

const SOURCE_TYPE_BY_EVENT_TYPE: Record<string, string> = {
	mcp_call: 'mcp',
	local_agent_tool_call: 'local_agent_tool_call'
};

export function normalizeSourceTypes(sourceTypes: readonly string[] | undefined): string[] {
	return ALL_SOURCE_TYPES.filter((sourceType) => sourceTypes?.includes(sourceType));
}

// Read the log sources out of an `event_type` query param handed over from the audit-logs page.
// Returns undefined when the param is absent or names no known source, meaning "caller said
// nothing" — the form keeps its own default rather than falling back to a single source here.
export function sourceTypesFromEventTypeParam(eventType: string | null): string[] | undefined {
	if (!eventType) return undefined;

	const sourceTypes = normalizeSourceTypes(
		eventType.split(',').flatMap((value) => SOURCE_TYPE_BY_EVENT_TYPE[value.trim()] ?? [])
	);
	return sourceTypes.length > 0 ? sourceTypes : undefined;
}

// Convert selected source types back into the `event_type` value the filter-options API expects.
export function eventTypeParamFromSourceTypes(sourceTypes: readonly string[]): string {
	return sourceTypes.map((source) => (source === 'mcp' ? 'mcp_call' : source)).join(',');
}

// Advanced Options offer values scoped to the selected log sources, so changing the sources
// invalidates every selection. The free-text query is source-independent and is kept.
export function clearSourceScopedFilters(filters: Record<string, unknown>) {
	for (const key of Object.keys(filters)) {
		if (key !== 'query') {
			filters[key] = '';
		}
	}
}

// The source-agnostic ("common") filter keys. These resolve to the correct column per source and
// are the only filters the backend accepts when more than one source is selected.
export const COMMON_FILTER_KEYS = [
	'actor',
	'operation',
	'mcp_server',
	'tool',
	'outcome',
	'client'
] as const;

// Decide which filter fields/rows should be visible for the currently selected source(s). The
// backend enforces the same split, so the form only ever shows filters it will accept:
//   - common filters        -> only when more than one source is selected
//   - source-specific       -> only when that single source is the sole selection
//   - shared columns        -> user_id/session_id/client_ip; only for a single source of either kind
export function filterVisibleExportFields<TField extends { filterKey: string }>(
	form: { sourceTypes: readonly string[] },
	fields: readonly TField[],
	commonFilterKeys: ReadonlySet<string>,
	mcpFilterKeys: ReadonlySet<string>,
	localFilterKeys: ReadonlySet<string>
): TField[] {
	const singleSource = form.sourceTypes.length === 1;
	const onlyMCP = singleSource && form.sourceTypes.includes('mcp');
	const onlyLocal = singleSource && form.sourceTypes.includes('local_agent_tool_call');
	const multiSource = form.sourceTypes.length > 1;

	return fields.filter((field) => {
		if (commonFilterKeys.has(field.filterKey)) {
			return multiSource;
		}
		if (mcpFilterKeys.has(field.filterKey)) {
			return onlyMCP;
		}
		if (localFilterKeys.has(field.filterKey)) {
			return onlyLocal;
		}
		// Shared columns (user_id, session_id, client_ip) are valid for a single source of either kind.
		return singleSource;
	});
}

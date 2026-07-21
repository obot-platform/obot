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

// Decide whether a single filter is usable for the currently selected source(s). The backend
// enforces the same split, so the forms only ever show (and fetch options for) filters it accepts:
//   - common filters        -> only when more than one source is selected
//   - source-specific       -> only when that single source is the sole selection
//   - shared columns        -> user_id/session_id/client_ip; only for a single source of either kind
export function isExportFilterKeyVisible(
	sourceTypes: readonly string[],
	filterKey: string,
	commonFilterKeys: ReadonlySet<string>,
	mcpFilterKeys: ReadonlySet<string>,
	localFilterKeys: ReadonlySet<string>
): boolean {
	const singleSource = sourceTypes.length === 1;

	if (commonFilterKeys.has(filterKey)) return sourceTypes.length > 1;
	if (mcpFilterKeys.has(filterKey)) return singleSource && sourceTypes.includes('mcp');
	if (localFilterKeys.has(filterKey))
		return singleSource && sourceTypes.includes('local_agent_tool_call');
	// Shared columns (user_id, session_id, client_ip) are valid for a single source of either kind.
	return singleSource;
}

// Filter the fields/rows the form renders down to the ones visible for the selected source(s).
export function filterVisibleExportFields<TField extends { filterKey: string }>(
	form: { sourceTypes: readonly string[] },
	fields: readonly TField[],
	commonFilterKeys: ReadonlySet<string>,
	mcpFilterKeys: ReadonlySet<string>,
	localFilterKeys: ReadonlySet<string>
): TField[] {
	return fields.filter((field) =>
		isExportFilterKeyVisible(
			form.sourceTypes,
			field.filterKey,
			commonFilterKeys,
			mcpFilterKeys,
			localFilterKeys
		)
	);
}

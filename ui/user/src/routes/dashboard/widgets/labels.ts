import { m } from '$lib/i18n';
import type { DeviceStatId, PlatformStatId, WidgetId } from '$lib/services/dashboard/layout';

export function widgetLabel(id: WidgetId): string {
	switch (id) {
		case 'platform-stats':
			return m.dashboard_on_platform();
		case 'device-scan-stats':
			return m.dashboard_device_scans();
		case 'server-activity':
			return m.dashboard_server_activity();
		case 'top-device-skills':
			return m.dashboard_top_device_skills();
		case 'most-deployed':
			return m.dashboard_most_deployed_servers();
		case 'device-clients':
			return m.dashboard_device_clients();
		case 'top-device-mcps':
			return m.dashboard_top_device_mcp_servers();
		case 'top-servers-used':
			return m.dashboard_top_servers_used();
		case 'device-scan-timeline':
			return m.inventory_enforcement_devices_scan_timeline();
		case 'token-usage':
			return m.audit_usage_usage_tokens_title();
		case 'popular-tools':
			return m.dashboard_recently_popular_tools();
		case 'tool-response-time':
			return m.dashboard_tool_call_avg_response_time();
		case 'model-stats':
			return m.dashboard_model_stats();
		case 'llm-requests':
			return m.audit_usage_usage_requests_over_time();
		case 'spend-by-provider':
			return m.audit_usage_usage_spend_by_provider();
		case 'top-models':
			return m.dashboard_top_models();
		case 'most-frequent-tool-calls':
			return m.audit_usage_usage_most_frequent_tool_calls();
		case 'tool-individual-response-time':
			return m.audit_usage_usage_individual_response_time();
		case 'tool-call-errors':
			return m.audit_usage_usage_tool_call_errors();
		case 'tool-call-errors-by-server':
			return m.audit_usage_usage_errors_by_server();
		case 'most-active-users':
			return m.audit_usage_usage_most_active_users();
	}
}

export function platformStatLabel(id: PlatformStatId): string {
	switch (id) {
		case 'total-users':
			return m.dashboard_total_users();
		case 'active-users':
			return m.dashboard_active_users();
		case 'total-tokens':
			return m.dashboard_total_tokens();
		case 'total-spend':
			return m.dashboard_total_spend();
	}
}

export { modelStatLabel } from '$lib/components/admin/token-usage/utils';

export function deviceStatLabel(id: DeviceStatId): string {
	switch (id) {
		case 'device-overview':
			return m.dashboard_unique_devices();
		case 'device-clients':
			return m.dashboard_unique_clients();
		case 'device-mcps':
			return m.dashboard_unique_mcps();
		case 'device-skills':
			return m.dashboard_unique_skills();
	}
}

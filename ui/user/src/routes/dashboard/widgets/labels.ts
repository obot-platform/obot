import { m } from '$lib/i18n';
import type { DeviceStatId, PlatformStatId, WidgetId } from '../dashboardLayout';

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
	}
}

export function platformStatLabel(id: PlatformStatId): string {
	switch (id) {
		case 'total-users':
			return m.dashboard_total_users();
		case 'monthly-active-users':
			return m.dashboard_monthly_active_users();
		case 'total-tokens':
			return m.dashboard_total_tokens();
		case 'total-spend':
			return m.dashboard_total_spend();
	}
}

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

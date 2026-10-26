import type { ReadonlyURL } from '$app/state';
import {
	DEFAULT_MCP_CATALOG_ID,
	MCP_CONNECTORS_NAV_SOURCE,
	MCP_NAV_SOURCE_PARAM
} from '$lib/constants';
import { m } from '$lib/i18n';
import {
	AdminService,
	UserService,
	type Fetcher,
	type LaunchServerType,
	type MCPFilterKind,
	type MCPFilterManifest,
	type Profile
} from '$lib/services';

export function isConnectorsNavigation(url: URL | ReadonlyURL) {
	return url.searchParams.get(MCP_NAV_SOURCE_PARAM) === MCP_CONNECTORS_NAV_SOURCE;
}

export function getCreatedEntryUrl(id: string, type?: LaunchServerType, message?: string) {
	if (type === 'multi') {
		return `/mcp-servers/s/${id}`;
	}

	const configureOauth = message === 'requires-oauth-config';
	return `/mcp-servers/c/${id}${configureOauth ? `?configure-oauth=true` : ''}`;
}

function useAdminEndpoints(url: URL, profile: Profile) {
	return Boolean(profile.hasAdminAccess?.()) && !isConnectorsNavigation(url);
}

export function getMCPCatalogServer(id: string, url: URL, profile: Profile, fetch: Fetcher) {
	if (useAdminEndpoints(url, profile)) {
		const wid = url.searchParams.get('wid');
		if (wid) {
			return UserService.getWorkspaceMCPCatalogServer(wid, id, { fetch });
		}
		return AdminService.getMCPCatalogServer(DEFAULT_MCP_CATALOG_ID, id, { fetch });
	}
	return UserService.getMcpCatalogServer(id, { fetch });
}

export function getMCPCatalogEntry(id: string, url: URL, profile: Profile, fetch: Fetcher) {
	if (useAdminEndpoints(url, profile)) {
		const wid = url.searchParams.get('wid');
		if (wid) {
			return UserService.getWorkspaceMCPCatalogEntry(wid, id, { fetch });
		}
		return AdminService.getMCPCatalogEntry(DEFAULT_MCP_CATALOG_ID, id, { fetch });
	}
	return UserService.getMCP(id, { fetch });
}

export function getSingleOrRemoteMcpServer(
	mcpServerId: string,
	catalogEntryId: string,
	url: URL,
	profile: Profile,
	fetch: Fetcher
) {
	const wid = url.searchParams.get('wid');
	if (useAdminEndpoints(url, profile) && wid) {
		return UserService.getWorkspaceCatalogEntryServer(wid, catalogEntryId, mcpServerId, {
			fetch
		});
	}
	return UserService.getSingleOrRemoteMcpServer(mcpServerId, { fetch });
}

// Filter kinds stay in English in table rows, because filters are saved in URLs;
// mcpFilterKindDisplay translates them only for display.
export const MCP_FILTER_KIND_LABELS: Record<MCPFilterKind, string> = {
	guardrail: 'Guardrail',
	webhook: 'Webhook',
	builtin: 'Built-in',
	mcpServer: 'MCP server'
};

const MCP_FILTER_KIND_DISPLAY: Record<MCPFilterKind, () => string> = {
	guardrail: () => m.mcps_filters_kind_guardrail(),
	webhook: () => m.mcps_filters_kind_webhook(),
	builtin: () => m.mcps_filters_kind_builtin(),
	mcpServer: () => m.mcps_filters_kind_mcp_server()
};

/** Translates a filter kind label from `MCP_FILTER_KIND_LABELS` for display. */
export function mcpFilterKindDisplay(label: unknown): string {
	const kind = (Object.keys(MCP_FILTER_KIND_LABELS) as MCPFilterKind[]).find(
		(k) => MCP_FILTER_KIND_LABELS[k] === label
	);
	return kind ? MCP_FILTER_KIND_DISPLAY[kind]() : String(label ?? '');
}

/** Classifies a filter by what backs it: guardrails, a webhook, a built-in or an MCP server. */
export function getMCPFilterKind(filter: MCPFilterManifest): MCPFilterKind {
	if (filter.guardrail) return 'guardrail';
	if (filter.systemMCPServerCatalogEntryID) return 'builtin';
	if (filter.url) return 'webhook';
	if (filter.mcpServerManifest) return 'mcpServer';
	return 'webhook';
}

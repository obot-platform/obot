import {
	DEFAULT_MCP_CATALOG_ID,
	MCP_CONNECTORS_NAV_SOURCE,
	MCP_NAV_SOURCE_PARAM
} from '$lib/constants';
import { AdminService, UserService, type Fetcher, type Profile } from '$lib/services';

export function isConnectorsNavigation(url: URL) {
	return url.searchParams.get(MCP_NAV_SOURCE_PARAM) === MCP_CONNECTORS_NAV_SOURCE;
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

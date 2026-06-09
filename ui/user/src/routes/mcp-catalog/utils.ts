import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
import { AdminService, UserService, type Fetcher, type Profile } from '$lib/services';

export function getMCPCatalogServer(
	id: string,
	wid: string | undefined | null,
	profile: Profile,
	fetch: Fetcher
) {
	if (profile.hasAdminAccess?.()) {
		if (wid) {
			return UserService.getWorkspaceMCPCatalogServer(wid, id, { fetch });
		}
		return AdminService.getMCPCatalogServer(DEFAULT_MCP_CATALOG_ID, id, { fetch });
	}
	return UserService.getMcpCatalogServer(id, { fetch });
}

export function getMCPCatalogEntry(
	id: string,
	wid: string | undefined | null,
	profile: Profile,
	fetch: Fetcher
) {
	if (profile.hasAdminAccess?.()) {
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
	wid: string | undefined | null,
	profile: Profile,
	fetch: Fetcher
) {
	if (profile.hasAdminAccess?.() && wid) {
		return UserService.getWorkspaceCatalogEntryServer(wid, catalogEntryId, mcpServerId, {
			fetch
		});
	}
	return UserService.getSingleOrRemoteMcpServer(mcpServerId, { fetch });
}

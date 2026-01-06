import { handleRouteError } from '$lib/errors';
import { ApiKeysService, ChatService } from '$lib/services';
import type { APIKey } from '$lib/services/api-keys/types';
import type { MCPCatalogServer } from '$lib/services/chat/types';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	let apiKeys: APIKey[] = [];
	let mcpServers: MCPCatalogServer[] = [];

	try {
		// Load all types of MCP servers the user has access to:
		// 1. Multi-user servers from default catalog and user workspaces
		// 2. User's deployed single-user/remote servers
		const [keys, catalogServers, deployedServers] = await Promise.all([
			ApiKeysService.listApiKeys({ fetch }),
			ChatService.listMCPCatalogServers({ fetch }),
			ChatService.listSingleOrRemoteMcpServers({ fetch })
		]);

		apiKeys = keys;

		// Merge and deduplicate servers
		const serverMap = new Map<string, MCPCatalogServer>();
		for (const server of [...catalogServers, ...deployedServers]) {
			if (!server.deleted) {
				serverMap.set(server.id, server);
			}
		}
		mcpServers = Array.from(serverMap.values());
	} catch (err) {
		handleRouteError(err, '/keys', profile.current);
	}

	return { apiKeys, mcpServers };
};

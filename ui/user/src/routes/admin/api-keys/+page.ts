import { handleRouteError } from '$lib/errors';
import { AdminService, ApiKeysService, ChatService } from '$lib/services';
import type { OrgUser } from '$lib/services/admin/types';
import type { APIKey } from '$lib/services/api-keys/types';
import type { MCPCatalogServer } from '$lib/services/chat/types';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	let myApiKeys: APIKey[] = [];
	let allApiKeys: APIKey[] = [];
	let users: OrgUser[] = [];
	let mcpServers: MCPCatalogServer[] = [];

	try {
		// Load all types of MCP servers:
		// 1. Multi-user servers from default catalog and user workspaces
		// 2. Admin's own deployed single-user/remote servers
		const [keys, allKeys, userList, catalogServers, deployedServers] = await Promise.all([
			ApiKeysService.listApiKeys({ fetch }),
			ApiKeysService.listAllApiKeys({ fetch }),
			AdminService.listUsers({ fetch }),
			ChatService.listMCPCatalogServers({ fetch }),
			ChatService.listSingleOrRemoteMcpServers({ fetch })
		]);

		myApiKeys = keys;
		allApiKeys = allKeys;
		users = userList;

		// Merge and deduplicate servers
		const serverMap = new Map<string, MCPCatalogServer>();
		for (const server of [...catalogServers, ...deployedServers]) {
			if (!server.deleted) {
				serverMap.set(server.id, server);
			}
		}
		mcpServers = Array.from(serverMap.values());
	} catch (err) {
		handleRouteError(err, '/admin/api-keys', profile.current);
	}

	return { myApiKeys, allApiKeys, users, mcpServers };
};

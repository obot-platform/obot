import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
import {
	AdminService,
	ChatService,
	type MCPCatalogEntry,
	type MCPCatalogServer
} from '$lib/services';
import { profile } from '.';

interface McpServerAndEntries {
	entries: MCPCatalogEntry[];
	servers: MCPCatalogServer[];
	userConfiguredServers: MCPCatalogServer[];
	loading: boolean;
	lastFetched: number | null;
	isInitialized: boolean;
}
const store = $state<{
	current: McpServerAndEntries;
	refreshAll: () => void;
	refreshUserConfiguredServers: () => void;
	initialize: (forceRefresh?: boolean) => void;
	fetchData: (forceRefresh?: boolean) => Promise<void>;
}>({
	current: {
		entries: [],
		servers: [],
		userConfiguredServers: [],
		loading: false,
		lastFetched: null,
		isInitialized: false
	},
	refreshAll,
	refreshUserConfiguredServers,
	initialize,
	fetchData
});

async function fetchData(forceRefresh = false) {
	if (store.current.loading) return;

	const now = Date.now();
	const cacheAge = 5 * 60 * 1000; // 5 minutes cache

	// Return cached data if it's fresh and not forcing refresh
	if (!forceRefresh && store.current.isInitialized && cacheAge > 0) {
		if (store.current.lastFetched && now - store.current.lastFetched < cacheAge) {
			return;
		}
	}

	store.current.loading = true;

	try {
		let entries: MCPCatalogEntry[] = [];
		let servers: MCPCatalogServer[] = [];
		let userConfiguredServers: MCPCatalogServer[] = [];

		if (profile.current.hasAdminAccess?.()) {
			const [adminEntries, adminServers, workspaceEntries, workspaceServers, ownConfiguredServers] =
				await Promise.all([
					AdminService.listMCPCatalogEntries(DEFAULT_MCP_CATALOG_ID, { all: true }),
					AdminService.listMCPCatalogServers(DEFAULT_MCP_CATALOG_ID, { all: true }),
					AdminService.listAllUserWorkspaceCatalogEntries(),
					AdminService.listAllUserWorkspaceMCPServers(),
					ChatService.listSingleOrRemoteMcpServers()
				]);
			entries = [...adminEntries, ...workspaceEntries];
			servers = [...adminServers, ...workspaceServers];
			userConfiguredServers = ownConfiguredServers;
		} else {
			const [ownConfiguredServers, entriesResult, serversResult] = await Promise.all([
				ChatService.listSingleOrRemoteMcpServers(),
				ChatService.listMCPs(),
				ChatService.listMCPCatalogServers()
			]);

			entries = entriesResult;
			servers = serversResult;
			userConfiguredServers = [...serversResult, ...ownConfiguredServers].filter(
				(server, index, self) => index === self.findIndex((t) => t.id === server.id)
			);
		}

		store.current = {
			entries,
			servers,
			userConfiguredServers,
			loading: false,
			lastFetched: now,
			isInitialized: true
		};
	} catch (error) {
		console.error('Failed to fetch mcp server, entries, and user configured servers:', error);
		store.current.loading = false;
	}
}

function refreshAll() {
	fetchData(true);
}

function initialize(forceRefresh = false) {
	fetchData(forceRefresh);
}

async function refreshUserConfiguredServers() {
	const response = await ChatService.listSingleOrRemoteMcpServers();
	store.current = {
		...store.current,
		userConfiguredServers: response
	};
}

export default store;

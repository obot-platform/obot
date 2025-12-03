import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
import {
	AdminService,
	ChatService,
	type MCPCatalogEntry,
	type MCPCatalogServer,
	type MCPServerInstance
} from '$lib/services';
import { profile } from '.';

interface McpServerAndEntries {
	entries: MCPCatalogEntry[];
	servers: MCPCatalogServer[];
	userInstances: MCPServerInstance[];
	userConfiguredServers: MCPCatalogServer[];
	loading: boolean;
	lastFetched: number | null;
	isInitialized: boolean;
}
const store = $state<{
	current: McpServerAndEntries;
	refreshAll: () => void;
	refreshUserConfiguredServers: () => void;
	refreshUserInstances: () => void;
	initialize: (forceRefresh?: boolean) => void;
	fetchData: (forceRefresh?: boolean) => Promise<void>;
}>({
	current: {
		entries: [],
		servers: [],
		userInstances: [],
		userConfiguredServers: [],
		loading: false,
		lastFetched: null,
		isInitialized: false
	},
	refreshAll,
	refreshUserConfiguredServers,
	refreshUserInstances,
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
		let userInstances: MCPServerInstance[] = [];

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
			userInstances = await ChatService.listMcpServerInstances();
			userConfiguredServers = ownConfiguredServers;
		} else {
			const [ownConfiguredServers, entriesResult, serversResult] = await Promise.all([
				ChatService.listSingleOrRemoteMcpServers(),
				ChatService.listMCPs(),
				ChatService.listMCPCatalogServers()
			]);

			entries = entriesResult;
			servers = serversResult;
			userInstances = await ChatService.listMcpServerInstances();
			userConfiguredServers = [...serversResult, ...ownConfiguredServers].filter(
				(server, index, self) => index === self.findIndex((t) => t.id === server.id)
			);
		}

		store.current = {
			entries,
			servers,
			userInstances,
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

async function refreshUserInstances() {
	const response = await ChatService.listMcpServerInstances();
	store.current = {
		...store.current,
		userInstances: response
	};
}

export default store;

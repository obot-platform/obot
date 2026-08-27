import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
import {
	AdminService,
	UserService,
	type MCPCatalogEntry,
	type MCPCatalogServer,
	type MCPServerInstance
} from '$lib/services';
import { profile } from '.';
import errors from './errors.svelte';

interface McpServerAndEntries {
	entries: MCPCatalogEntry[];
	servers: MCPCatalogServer[];
	userInstances: MCPServerInstance[];
	userConfiguredServers: MCPCatalogServer[];
	loading: boolean;
	lastFetched: number | null;
	isInitialized: boolean;
}

type MCPDataScope = 'user' | 'admin';

interface MCPDataOptions {
	forceRefresh?: boolean;
	scope?: MCPDataScope;
}

let loadedScope: MCPDataScope | undefined;
let fetchGeneration = 0;

const store = $state<{
	current: McpServerAndEntries;
	refreshAll: () => Promise<void>;
	refreshEntries: () => Promise<void>;
	refreshUserConfiguredServers: () => Promise<void>;
	refreshUserInstances: () => Promise<void>;
	removeServer: (serverID: string) => void;
	initialize: (options?: MCPDataOptions) => void;
	fetchData: (options?: MCPDataOptions) => Promise<void>;
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
	refreshEntries,
	refreshUserConfiguredServers,
	refreshUserInstances,
	removeServer,
	initialize,
	fetchData
});

function filterOutDuplicateAndDeleted(servers: MCPCatalogServer[]) {
	return servers.filter(
		(server, index, self) => index === self.findIndex((t) => t.id === server.id) && !server.deleted
	);
}

function setCanConnectAndFilterDeleted(
	entries: MCPCatalogEntry[],
	userScopedEntries: MCPCatalogEntry[]
) {
	const accessibleEntryIds = new Set(userScopedEntries.map((e) => e.id));
	return entries
		.filter((entry) => !entry.deleted)
		.map((entry) => ({
			...entry,
			canConnect: accessibleEntryIds.has(entry.id)
		}));
}

async function fetchData({ forceRefresh = false, scope = 'admin' }: MCPDataOptions = {}) {
	const generation = ++fetchGeneration;
	const now = Date.now();
	const cacheAge = 5 * 60 * 1000; // 5 minutes cache

	// Return cached data if it's fresh and not forcing refresh
	if (
		!store.current.loading &&
		!forceRefresh &&
		loadedScope === scope &&
		store.current.isInitialized &&
		cacheAge > 0
	) {
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

		if (scope === 'admin' && profile.current.hasAdminAccess?.()) {
			const [
				adminEntries,
				adminServers,
				workspaceEntries,
				workspaceServers,
				ownConfiguredServers,
				userScopedEntries,
				userScopedServers
			] = await Promise.all([
				AdminService.listMCPCatalogEntries(DEFAULT_MCP_CATALOG_ID, { all: true }),
				AdminService.listMCPCatalogServers(DEFAULT_MCP_CATALOG_ID, { all: true }),
				AdminService.listAllUserWorkspaceCatalogEntries(),
				AdminService.listAllUserWorkspaceMCPServers(),
				UserService.listSingleOrRemoteMcpServers(),
				UserService.listMCPs({ minimal: true }),
				UserService.listMCPCatalogServers()
			]);

			// Create sets of IDs the admin has access to via ACRs
			const accessibleServerIds = new Set(userScopedServers.map((s) => s.id));

			entries = setCanConnectAndFilterDeleted(
				[...adminEntries, ...workspaceEntries],
				userScopedEntries
			);
			servers = [...adminServers, ...workspaceServers].map((server) => ({
				...server,
				canConnect: accessibleServerIds.has(server.id)
			}));
			userInstances = await UserService.listMcpServerInstances();
			userConfiguredServers = filterOutDuplicateAndDeleted([...servers, ...ownConfiguredServers]);
		} else {
			const userScopedServersPromise = UserService.listMCPCatalogServers();
			const [ownConfiguredServers, entriesResult, userScopedServers, serversResult] =
				await Promise.all([
					UserService.listSingleOrRemoteMcpServers(),
					UserService.listMCPs({ minimal: true }),
					userScopedServersPromise,
					profile.current.hasAdminAccess?.()
						? AdminService.listMCPCatalogServers(DEFAULT_MCP_CATALOG_ID, { all: true })
						: userScopedServersPromise
				]);

			entries = entriesResult
				.filter((entry) => !entry.deleted)
				.map((entry) => ({ ...entry, canConnect: true }));
			const accessibleServerIds = new Set(userScopedServers.map((server) => server.id));
			servers = serversResult.map((server) => ({
				...server,
				canConnect: accessibleServerIds.has(server.id)
			}));
			userInstances = await UserService.listMcpServerInstances();
			userConfiguredServers = filterOutDuplicateAndDeleted([...servers, ...ownConfiguredServers]);
		}
		if (generation !== fetchGeneration) {
			return;
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
		loadedScope = scope;
	} catch (error) {
		if (generation !== fetchGeneration) {
			return;
		}
		errors.append(error);
		store.current.loading = false;
	}
}

async function refreshAll() {
	await fetchData({ forceRefresh: true, scope: loadedScope });
}

async function initialize(options: MCPDataOptions = {}) {
	await fetchData(options);
}

async function refreshEntries() {
	try {
		if (loadedScope !== 'user' && profile.current.hasAdminAccess?.()) {
			const [adminEntries, workspaceEntries, userScopedEntries] = await Promise.all([
				AdminService.listMCPCatalogEntries(DEFAULT_MCP_CATALOG_ID, { all: true }),
				AdminService.listAllUserWorkspaceCatalogEntries(),
				UserService.listMCPs({ minimal: true })
			]);
			store.current = {
				...store.current,
				entries: setCanConnectAndFilterDeleted(
					[...adminEntries, ...workspaceEntries],
					userScopedEntries
				)
			};
		} else {
			const entries = await UserService.listMCPs({ minimal: true });
			store.current = {
				...store.current,
				entries: entries.filter((entry) => !entry.deleted)
			};
		}
	} catch (error) {
		errors.append(error);
	}
}

async function refreshUserConfiguredServers() {
	const ownConfiguredServers = await UserService.listSingleOrRemoteMcpServers();
	const userConfiguredServers = filterOutDuplicateAndDeleted([
		...store.current.servers,
		...ownConfiguredServers
	]);

	store.current = {
		...store.current,
		userConfiguredServers
	};
}

async function refreshUserInstances() {
	const response = await UserService.listMcpServerInstances();
	store.current = {
		...store.current,
		userInstances: response
	};
}

function removeServer(serverID: string) {
	store.current = {
		...store.current,
		servers: store.current.servers.filter((server) => server.id !== serverID),
		userConfiguredServers: store.current.userConfiguredServers.filter(
			(server) => server.id !== serverID
		),
		userInstances: store.current.userInstances.filter(
			(instance) => instance.mcpServerID !== serverID
		)
	};
}

export default store;

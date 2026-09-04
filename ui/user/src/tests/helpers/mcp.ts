import type {
	MCPCatalogEntry,
	MCPCatalogEntryFieldManifest,
	MCPCatalogEntryServerManifest,
	MCPCatalogServer,
	MCPServer,
	Runtime
} from '$lib/services';
import { faker } from '@faker-js/faker';

function baseServerManifest(
	overrides: Partial<MCPServer> & { runtime: Runtime; name: string }
): MCPServer {
	const { runtime, name, ...rest } = overrides;
	const manifest: MCPServer = {
		name,
		shortDescription: `${name} short description`,
		description: '',
		icon: '',
		runtime,
		...rest
	};

	if (runtime === 'npx' && !manifest.npxConfig) {
		manifest.npxConfig = {
			package: '@modelcontextprotocol/server-everything',
			args: [],
			egressDomains: []
		};
	}
	if (runtime === 'remote' && !manifest.remoteConfig) {
		manifest.remoteConfig = {
			url: 'https://example.com/mcp'
		};
	}
	if (runtime === 'composite' && !manifest.compositeConfig) {
		manifest.compositeConfig = { componentServers: [] };
	}

	return manifest;
}

function baseEntryManifest(
	overrides: Partial<MCPCatalogEntryServerManifest> & {
		runtime: Runtime;
		name: string;
	}
): MCPCatalogEntryServerManifest {
	const { runtime, name, ...rest } = overrides;
	const manifest: MCPCatalogEntryServerManifest = {
		name,
		shortDescription: `${name} short description`,
		description: '',
		icon: '',
		runtime,
		...rest
	};

	if (runtime === 'npx' && !manifest.npxConfig) {
		manifest.npxConfig = {
			package: '@modelcontextprotocol/server-everything',
			args: [],
			egressDomains: []
		};
	}
	if (runtime === 'remote' && !manifest.remoteConfig) {
		manifest.remoteConfig = {
			fixedURL: 'https://example.com/mcp'
		};
	}
	return manifest;
}

export function createMCPCatalogEntry(
	overrides: Partial<Omit<MCPCatalogEntry, 'manifest'>> & {
		id: string;
		name: string;
		runtime?: Runtime;
		env?: MCPCatalogEntryFieldManifest[];
		manifest?: Partial<MCPCatalogEntryServerManifest>;
	}
): MCPCatalogEntry {
	const { id, name, runtime = 'npx', env, manifest: manifestOverrides, ...rest } = overrides;

	return {
		id,
		created: faker.date.past().toISOString(),
		type: 'mcpservercatalogentry',
		isCatalogEntry: true,
		manifest: baseEntryManifest({
			name,
			runtime,
			...(env
				? {
						config: env.map(({ file, dynamicFile, interpolated, ...field }) => ({
							...field,
							usage: interpolated
								? ('interpolated' as const)
								: file
									? dynamicFile
										? ('dynamicFile' as const)
										: ('file' as const)
									: ('env' as const)
						}))
					}
				: {}),
			...manifestOverrides
		}),
		...rest
	};
}

export function createMCPCatalogServer(
	overrides: Partial<Omit<MCPCatalogServer, 'manifest'>> & {
		id: string;
		name: string;
		runtime?: Runtime;
		serverUserType?: 'singleUser' | 'multiUser';
		catalogEntryID?: string;
		env?: MCPCatalogEntryFieldManifest[];
		manifest?: Partial<MCPServer>;
		userID: string;
	}
): MCPCatalogServer {
	const {
		id,
		name,
		runtime = 'npx',
		serverUserType = 'singleUser',
		catalogEntryID = '',
		env,
		manifest: manifestOverrides,
		configured = true,
		...rest
	} = overrides;

	return {
		id,
		configured,
		catalogEntryID,
		missingRequiredEnvVars: [],
		mcpCatalogID: 'default',
		created: faker.date.past().toISOString(),
		updated: faker.date.recent().toISOString(),
		type: 'mcpserver',
		serverUserType,
		manifest: baseServerManifest({
			name,
			runtime,
			...(env
				? {
						config: env.map(({ file, dynamicFile, interpolated, ...field }) => ({
							...field,
							usage: interpolated
								? ('interpolated' as const)
								: file
									? dynamicFile
										? ('dynamicFile' as const)
										: ('file' as const)
									: ('env' as const)
						}))
					}
				: {}),
			...manifestOverrides
		}),
		...rest
	};
}

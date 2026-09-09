import type {
	MCPCatalogEntry,
	MCPCatalogEntryFieldManifest,
	MCPCatalogEntryServerManifest,
	MCPCatalogServer,
	MCPServer,
	Runtime,
	VMCP,
	VMCPComponent,
	VMCPProfile
} from '$lib/services';
import { faker } from '@faker-js/faker';

/**
 * Test-only representation of the first-class vMCP API resource.
 *
 * Keep this deliberately independent from the catalog-entry helper below. A
 * vMCP is not a composite catalog entry: its display metadata is flattened at
 * the top level and every component owns a cached catalog-entry snapshot.
 * Defining the fixture shape here also lets API/UI tests assert the wire
 * format while the generated client types evolve.
 */
export type VMCPTestComponent = VMCPComponent;
export type VMCPTestProfile = VMCPProfile;

export type VMCPTestResource = VMCP;

/** Copy a catalog manifest into the snapshot shape used by vMCPs. */
export function createVMCPComponent(
	entry: MCPCatalogEntry,
	overrides: Partial<VMCPTestComponent> = {}
): VMCPTestComponent {
	const manifest = { ...entry.manifest };
	return {
		id: `component-${entry.id}`,
		name: entry.manifest.name ?? entry.id,
		mcpCatalogID: 'default',
		mcpServerCatalogEntryID: entry.id,
		catalogEntry: {
			manifest,
			unsupportedTools: []
		},
		...overrides
	} as VMCPTestComponent;
}

/** Build a first-class vMCP resource around one or more source catalog entries. */
export function createVMCP(
	overrides: Partial<VMCPTestResource> & { id?: string; displayName?: string } = {},
	entries: MCPCatalogEntry[] = []
): VMCPTestResource {
	const sourceEntries =
		entries.length > 0
			? entries
			: [
					createMCPCatalogEntry({
						id: 'entry-default',
						name: 'Default'
					})
				];
	const components =
		overrides.components ?? sourceEntries.map((entry) => createVMCPComponent(entry));
	return {
		id: overrides.id ?? 'vmcp-1',
		created: '2026-01-01T00:00:00.000Z',
		type: 'vmcp',
		displayName: overrides.displayName ?? 'Issue Tracker vMCP',
		description: 'A first-class virtual MCP server',
		icon: '',
		profiles: [
			{
				name: 'default',
				subjects: [{ type: 'selector', id: '*' }],
				allowAllTools: true
			}
		],
		forceSingleUser: false,
		userID: '',
		status: {
			ready: true,
			components: components.map((component) => ({ name: component.name, ready: true }))
		},
		links: { connectURL: `/mcp-connect/${overrides.id ?? 'vmcp-1'}` },
		...overrides,
		components
	} as VMCPTestResource;
}

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

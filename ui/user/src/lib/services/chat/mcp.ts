import type { ConnectedServer } from '$lib/components/mcp/MyMcpServers.svelte';
import {
	ChatService,
	type MCPCatalogEntry,
	type MCPCatalogServer,
	type MCPServer,
	type MCPSubField,
	type Project
} from '..';

export interface MCPServerInfo extends MCPServer {
	id?: string;
	env?: (MCPSubField & { value: string; custom?: string })[];
	headers?: (MCPSubField & { value: string; custom?: string })[];
	manifest?: MCPServer;
}

export async function createProjectMcp(project: Project, mcpId: string, alias?: string) {
	return await ChatService.createProjectMCP(project.assistantID, project.id, mcpId, alias);
}

export function isValidMcpConfig(mcpConfig: MCPServerInfo) {
	return (
		mcpConfig.env?.every((env) => !env.required || env.value) &&
		mcpConfig.headers?.every((header) => !header.required || header.value)
	);
}

export function isAuthRequiredBundle(bundleId?: string): boolean {
	if (!bundleId) return false;

	// List of bundle IDs that don't require authentication
	const nonRequiredAuthBundles = [
		'browser-bundle',
		'google-search-bundle',
		'images-bundle',
		'memory',
		'obot-search-bundle',
		'time',

		'die-roller',
		'proxycurl-bundle'
	];
	return !nonRequiredAuthBundles.includes(bundleId);
}

export function parseCategories(item?: MCPCatalogServer | MCPCatalogEntry | null) {
	if (!item) return [];
	return item.manifest.metadata
		? (item.manifest.metadata.categories?.split(',') ?? []).map((c) => c.trim()).filter((c) => c)
		: [];
}

export function convertEnvHeadersToRecord(
	envs: MCPServerInfo['env'],
	headers: MCPServerInfo['headers']
) {
	const secretValues: Record<string, string> = {};
	for (const env of envs ?? []) {
		if (env.value) {
			secretValues[env.key] = env.value;
		}
	}

	for (const header of headers ?? []) {
		if (header.value) {
			secretValues[header.key] = header.value;
		}
	}
	return secretValues;
}

export function hasEditableConfiguration(item: MCPCatalogEntry) {
	const hasUrlToFill =
		!item.manifest?.remoteConfig?.fixedURL && item.manifest?.remoteConfig?.hostname;
	const hasEnvsToFill = item.manifest?.env && item.manifest.env.length > 0;
	const hasHeadersToFill =
		item.manifest?.remoteConfig?.headers && item.manifest.remoteConfig?.headers.length > 0;

	return hasUrlToFill || hasEnvsToFill || hasHeadersToFill;
}

export function requiresUserUpdate(mcpServer?: ConnectedServer) {
	if (!mcpServer) return false;
	if (mcpServer.server?.needsURL) {
		return true;
	}
	return typeof mcpServer.server?.configured === 'boolean'
		? mcpServer.server?.configured === false
		: false;
}

function convertEntriesToTableData(entries: MCPCatalogEntry[] | undefined) {
	if (!entries) {
		return [];
	}

	return entries
		.filter((entry) => !entry.deleted)
		.map((entry) => {
			return {
				id: entry.id,
				name: entry.manifest?.name ?? '',
				icon: entry.manifest?.icon,
				source: entry.sourceURL || 'manual',
				data: entry,
				users: entry.userCount ?? 0,
				editable: !entry.sourceURL,
				type: entry.manifest.runtime === 'remote' ? 'remote' : 'single',
				created: entry.created
			};
		});
}

function convertServersToTableData(servers: MCPCatalogServer[] | undefined) {
	if (!servers) {
		return [];
	}

	return servers
		.filter((server) => !server.catalogEntryID && !server.deleted)
		.map((server) => {
			return {
				id: server.id,
				name: server.manifest.name ?? '',
				icon: server.manifest.icon,
				source: 'manual',
				type: 'multi',
				data: server,
				users: server.mcpServerInstanceUserCount ?? 0,
				editable: true,
				created: server.created
			};
		});
}

export function convertEntriesAndServersToTableData(
	entries: MCPCatalogEntry[],
	servers: MCPCatalogServer[]
) {
	const entriesTableData = convertEntriesToTableData(entries);
	const serversTableData = convertServersToTableData(servers);
	return [...entriesTableData, ...serversTableData];
}

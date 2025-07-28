import type { ConnectedServer } from '$lib/components/mcp/MyMcpServers.svelte';
import {
	ChatService,
	type MCPCatalogEntry,
	type MCPCatalogServer,
	type MCPInfo,
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

export async function createProjectMcp(project: Project, mcpId: string) {
	return await ChatService.createProjectMCP(project.assistantID, project.id, mcpId);
}

export function isValidMcpConfig(mcpConfig: MCPServerInfo) {
	return (
		mcpConfig.env?.every((env) => !env.required || env.value) &&
		mcpConfig.headers?.every((header) => !header.required || header.value)
	);
}

export function initMCPConfig(manifest?: MCPInfo | MCPServer): MCPServerInfo {
	return {
		...manifest,
		name: manifest?.name ?? '',
		description: manifest?.description ?? '',
		icon: manifest?.icon ?? '',
		env: manifest?.env?.map((e) => ({ ...e, value: '' })) ?? [],
		args: manifest?.args ? [...manifest.args] : [],
		command: manifest?.command ?? '',
		headers: manifest?.headers?.map((e) => ({ ...e, value: '' })) ?? []
	};
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
	if ('manifest' in item && item.manifest.metadata?.categories) {
		return item.manifest.metadata.categories.split(',') ?? [];
	}
	if ('commandManifest' in item && item.commandManifest?.metadata?.categories) {
		return item.commandManifest.metadata.categories.split(',') ?? [];
	}
	if ('urlManifest' in item && item.urlManifest?.metadata?.categories) {
		return item.urlManifest.metadata.categories.split(',') ?? [];
	}
	return [];
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
	const manifest = item.commandManifest ?? item.urlManifest;
	const hasUrlToFill = !manifest?.fixedURL && manifest?.hostname;
	const hasEnvsToFill = manifest?.env && manifest.env.length > 0;
	const hasHeadersToFill = manifest?.headers && manifest.headers.length > 0;

	return hasUrlToFill || hasEnvsToFill || hasHeadersToFill;
}

export function requiresUserUpdate(mcpServer?: ConnectedServer) {
	if (!mcpServer) return false;
	if (mcpServer.server?.needsURL) {
		return true;
	}
	return typeof mcpServer.server?.configured === 'boolean' ? mcpServer.server?.configured : false;
}

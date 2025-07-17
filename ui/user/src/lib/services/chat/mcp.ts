import {
	ChatService,
	type MCPCatalogEntry,
	type MCPCatalogServer,
	type MCPInfo,
	type MCPServer,
	type MCPSubField,
	type Project,
	type ProjectMCP
} from '..';

export interface MCPServerInfo extends MCPServer {
	id?: string;
	env?: (MCPSubField & { value: string; custom?: string })[];
	headers?: (MCPSubField & { value: string; custom?: string })[];
	manifest?: MCPServer;
}

export function getKeyValuePairs(customMcpConfig: MCPServerInfo) {
	return [...(customMcpConfig.env ?? []), ...(customMcpConfig.headers ?? [])].reduce<
		Record<string, string>
	>(
		(acc, item) => ({
			...acc,
			[item.key]: item.value
		}),
		{}
	);
}

export async function createProjectMcp(
	mcpServerInfo: MCPServerInfo,
	project: Project,
	mcpId?: string
) {
	const newProjectMcp = await ChatService.createProjectMCP(
		project.assistantID,
		project.id,
		mcpServerInfo,
		mcpId
	);

	// above handles creation of mcp server,
	// now configure the env/header values
	const keyValuePairs = getKeyValuePairs(mcpServerInfo);

	const configuredProjectMcp = await ChatService.configureProjectMCPEnvHeaders(
		project.assistantID,
		project.id,
		newProjectMcp.id,
		keyValuePairs
	);

	return configuredProjectMcp;
}

export async function updateProjectMcp(
	updatingMcpServerInfo: MCPServerInfo,
	projectMcpId: string,
	project: Project
) {
	const updatedProjectMcp = await ChatService.updateProjectMCP(
		project.assistantID,
		project.id,
		projectMcpId,
		updatingMcpServerInfo
	);

	const keyValuePairs = getKeyValuePairs(updatingMcpServerInfo);

	await ChatService.configureProjectMCPEnvHeaders(
		project.assistantID,
		project.id,
		projectMcpId,
		keyValuePairs
	);

	return updatedProjectMcp;
}

export function isValidMcpConfig(mcpConfig: MCPServerInfo) {
	return (
		mcpConfig.env?.every((env) => !env.required || env.value) &&
		mcpConfig.headers?.every((header) => !header.required || header.value)
	);
}

export function initMCPConfig(manifest?: MCPInfo | ProjectMCP | MCPServer): MCPServerInfo {
	const mcpServer = manifest && 'manifest' in manifest ? manifest.manifest : manifest;
	return {
		...mcpServer,
		name: mcpServer?.name ?? '',
		description: mcpServer?.description ?? '',
		icon: mcpServer?.icon ?? '',
		env: mcpServer?.env?.map((e) => ({ ...e, value: '' })) ?? [],
		args: mcpServer?.args ? [...mcpServer.args] : [],
		command: mcpServer?.command ?? '',
		headers: mcpServer?.headers?.map((e) => ({ ...e, value: '' })) ?? []
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

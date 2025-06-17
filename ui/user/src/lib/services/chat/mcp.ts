import {
	ChatService,
	type MCPInfo,
	type MCPServer,
	type MCPSubField,
	type Project,
	type ProjectMCP
} from '..';

export interface MCPServerInfo extends Omit<ProjectMCP, 'id' | 'type'> {
	id?: string;
	env?: (MCPSubField & { value: string; custom?: string })[];
	headers?: (MCPSubField & { value: string; custom?: string })[];
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
	if (manifest && 'server' in manifest) {
		return {
			...manifest.server,
			env: manifest.server.env?.map((e) => ({ ...e, value: '' })) ?? [],
			args: manifest.server.args ? [...manifest.server.args] : [],
			command: manifest.server.command ?? '',
			headers: manifest.server.headers?.map((e) => ({ ...e, value: '' })) ?? []
		};
	}

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

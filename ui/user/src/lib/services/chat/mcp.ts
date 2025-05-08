import { ChatService, type MCPManifest, type MCPSubField, type Project, type ProjectMCP } from '..';

export interface MCPServerInfo extends Omit<ProjectMCP, 'id'> {
	id?: string;
	env?: (MCPSubField & { value: string; custom?: boolean })[];
	headers?: (MCPSubField & { value: string; custom?: boolean })[];
}

function getKeyValuePairs(customMcpConfig: MCPServerInfo) {
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

export function initConfigFromManifest(manifest?: MCPManifest | ProjectMCP): MCPServerInfo {
	if (manifest && 'server' in manifest) {
		return {
			...manifest.server,
			env: manifest.server.env?.map((e) => ({ ...e, value: '', custom: false })) ?? [],
			args: manifest.server.args ? [...manifest.server.args] : [],
			command: manifest.server.command ?? '',
			headers: manifest.server.headers?.map((e) => ({ ...e, value: '', custom: false })) ?? []
		};
	}

	return {
		...manifest,
		name: manifest?.name ?? '',
		description: manifest?.description ?? '',
		icon: manifest?.icon ?? '',
		env: manifest?.env?.map((e) => ({ ...e, value: '', custom: false })) ?? [],
		args: manifest?.args ? [...manifest.args] : [],
		command: manifest?.command ?? '',
		headers: manifest?.headers?.map((e) => ({ ...e, value: '', custom: false })) ?? []
	};
}

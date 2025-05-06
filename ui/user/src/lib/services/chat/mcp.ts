import { ChatService, type MCPSubField, type Project, type ProjectMCP } from '..';

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

	await ChatService.configureProjectMCPEnvHeaders(
		project.assistantID,
		project.id,
		newProjectMcp.id,
		keyValuePairs
	);

	return newProjectMcp;
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

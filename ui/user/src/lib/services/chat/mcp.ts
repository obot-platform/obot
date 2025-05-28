import type { ToolBundleItem } from '$lib/context/toolReferences.svelte';
import {
	ChatService,
	type MCPInfo,
	type MCPServer,
	type MCPSubField,
	type Project,
	type ProjectCredential,
	type ProjectMCP
} from '..';

export interface MCPServerInfo extends Omit<ProjectMCP, 'id'> {
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
		'database',
		'die-roller',
		'proxycurl-bundle'
	];
	return !nonRequiredAuthBundles.includes(bundleId);
}

async function hasLocalConfig(
	mcp: ProjectMCP,
	project: Project,
	localCredentials: ProjectCredential[],
	toolBundleMap: Map<string, ToolBundleItem>
): Promise<boolean> {
	// Handle legacy tool bundles
	if (mcp.catalogID && toolBundleMap.get(mcp.catalogID)) {
		return localCredentials.some((cred) => cred.toolID === mcp.catalogID && cred.exists === true);
	}

	// Real MCP server, reveal any configured env headers
	let envHeaders: Record<string, string> = {};
	try {
		envHeaders = await ChatService.revealProjectMCPEnvHeaders(
			project.assistantID,
			project.id,
			mcp.id
		);
	} catch (err) {
		if (err instanceof Error && err.message.includes('404')) {
			return false;
		}
	}

	return Object.keys(envHeaders).length > 0;
}

function isNotConfigured(
	mcp: ProjectMCP,
	localCredentials: ProjectCredential[],
	inheritedCredentials: ProjectCredential[],
	toolBundleMap: Map<string, ToolBundleItem>,
	chatbot: boolean
) {
	if (!mcp.catalogID || !toolBundleMap.get(mcp.catalogID)) {
		return mcp.configured !== true;
	}

	const localCredential = localCredentials.find((cred) => cred.toolID === mcp.catalogID);

	if (localCredential === undefined) {
		// When there's no entry in this list, it means the tool does not require credentials.
		return false;
	}

	const hasLocalCredential = localCredential.exists;
	if (chatbot) {
		return !hasLocalCredential;
	}

	const hasInheritedCredential = inheritedCredentials.some(
		(cred) => cred.toolID === mcp.catalogID && cred.exists === true
	);

	return !(hasLocalCredential || hasInheritedCredential);
}

export async function fetchConfigurationStatuses(
	project: Project,
	projectMCPs: ProjectMCP[],
	toolBundleMap: Map<string, ToolBundleItem>,
	chatbot: boolean
) {
	if (!project?.assistantID || !project.id) return;

	try {
		const localCredentials = (
			await ChatService.listProjectLocalCredentials(project.assistantID, project.id)
		).items;

		const inheritedCredentials = (
			await ChatService.listProjectCredentials(project.assistantID, project.id)
		).items;

		const localConfigurations: Record<string, boolean> = {};
		for (const mcp of projectMCPs) {
			localConfigurations[mcp.id] = await hasLocalConfig(
				mcp,
				project,
				localCredentials,
				toolBundleMap
			);
		}

		const requiresConfiguration: Record<string, ProjectMCP> = {};
		for (const mcp of projectMCPs) {
			if (isNotConfigured(mcp, localCredentials, inheritedCredentials, toolBundleMap, chatbot)) {
				requiresConfiguration[mcp.id] = mcp;
			}
		}

		return {
			configured: localConfigurations,
			requiresConfiguration: requiresConfiguration
		};
	} catch (error) {
		console.error('Failed to fetch credentials:', error);
		return {
			configured: {},
			requiresConfiguration: {}
		};
	}
}

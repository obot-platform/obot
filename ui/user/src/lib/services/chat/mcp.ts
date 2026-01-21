import type { CompositeLaunchFormData } from '$lib/components/mcp/CatalogConfigureForm.svelte';
import { mcpServersAndEntries, profile } from '$lib/stores';
import { getUserDisplayName } from '$lib/utils';
import {
	ChatService,
	type AccessControlRule,
	type LaunchServerType,
	type MCPCatalogEntry,
	type MCPCatalogServer,
	type MCPServer,
	type MCPServerInstance,
	type MCPSubField,
	type OrgUser,
	type Project,
	type ProjectMCP
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
	// For composite servers, check if any component has editable configuration
	if (item.manifest?.runtime === 'composite') {
		const componentServers = item.manifest?.compositeConfig?.componentServers || [];
		return componentServers.some((component) => {
			const hasEnvs = component.manifest?.env && component.manifest.env.length > 0;
			const hasHeaders =
				(component?.manifest?.remoteConfig?.headers?.filter?.((header) => !header.value)?.length ??
					0) > 0;
			const hasUrlToFill =
				!component.manifest?.remoteConfig?.fixedURL && component.manifest?.remoteConfig?.hostname;
			return hasEnvs || hasHeaders || hasUrlToFill;
		});
	}

	const hasUrlToFill =
		!item.manifest?.remoteConfig?.fixedURL && item.manifest?.remoteConfig?.hostname;
	const hasEnvsToFill = item.manifest?.env && item.manifest.env.length > 0;
	const hasHeadersToFill =
		(item?.manifest?.remoteConfig?.headers?.filter?.((header) => !header.value)?.length ?? 0) > 0;

	return hasUrlToFill || hasEnvsToFill || hasHeadersToFill;
}

export function requiresUserUpdate(server?: MCPCatalogServer) {
	if (!server) return false;
	if (server?.needsURL) {
		return true;
	}
	return typeof server?.configured === 'boolean' ? server?.configured === false : false;
}

export function getUserRegistry(
	entity: MCPCatalogEntry | MCPCatalogServer | AccessControlRule,
	usersMap?: Map<string, OrgUser>
) {
	let registry: string = 'Global Registry';
	if (entity.powerUserWorkspaceID) {
		const userID = entity.powerUserWorkspaceID.split('-')?.pop() || '';
		registry =
			userID === profile.current.id
				? 'My Registry'
				: usersMap
					? `${getUserDisplayName(usersMap, userID)}'s Registry`
					: 'Unknown Registry';
	}
	return registry;
}

function convertEntriesToTableData(
	entries?: MCPCatalogEntry[],
	usersMap?: Map<string, OrgUser>,
	userConfiguredServers?: MCPCatalogServer[]
) {
	if (!entries) {
		return [];
	}

	const userConfiguredServersMap = userConfiguredServers
		? new Map(userConfiguredServers.map((server) => [server.catalogEntryID, server]))
		: undefined;

	return entries
		.filter((entry) => !entry.deleted)
		.map((entry) => {
			const registry = getUserRegistry(entry, usersMap);
			return {
				id: entry.id,
				name: entry.manifest?.name ?? '',
				icon: entry.manifest?.icon,
				data: entry,
				users: entry.userCount ?? 0,
				editable: !entry.sourceURL,
				type:
					entry.manifest.runtime === 'remote'
						? 'remote'
						: entry.manifest.runtime === 'composite'
							? 'composite'
							: 'single',
				created: entry.created,
				registry,
				needsUpdate: entry.needsUpdate,
				connected: userConfiguredServersMap?.has(entry.id)
			};
		});
}

function convertServersToTableData(
	servers?: MCPCatalogServer[],
	usersMap?: Map<string, OrgUser>,
	instances?: MCPServerInstance[]
) {
	if (!servers) {
		return [];
	}

	const instancesMap = instances
		? new Map(instances.map((instance) => [instance.mcpServerID, instance]))
		: undefined;

	return servers
		.filter((server) => !server.catalogEntryID && !server.deleted)
		.map((server) => {
			const registry = getUserRegistry(server, usersMap);
			return {
				id: server.id,
				name: server.manifest.name ?? '',
				icon: server.manifest.icon,
				source: 'manual',
				type: 'multi',
				data: server,
				users: server.mcpServerInstanceUserCount ?? 0,
				editable: true,
				created: server.created,
				registry,
				connected: instancesMap?.has(server.id)
			};
		});
}

export function convertEntriesAndServersToTableData(
	entries: MCPCatalogEntry[],
	servers: MCPCatalogServer[],
	usersMap?: Map<string, OrgUser>,
	userConfiguredServers?: MCPCatalogServer[],
	instances?: MCPServerInstance[]
) {
	const entriesTableData = convertEntriesToTableData(entries, usersMap, userConfiguredServers);
	const serversTableData = convertServersToTableData(servers, usersMap, instances);
	return [...entriesTableData, ...serversTableData];
}

export function getServerTypeLabel(server?: MCPCatalogServer | MCPCatalogEntry) {
	if (!server) return '';

	const runtime = server.manifest.runtime;
	if (runtime === 'remote') return 'Remote';
	if (runtime === 'composite') return 'Composite';

	if ('isCatalogEntry' in server) return 'Single User';

	const catalogEntryId = 'catalogEntryID' in server ? server.catalogEntryID : undefined;
	return catalogEntryId ? 'Single User' : 'Multi-User';
}

export function getServerTypeLabelByType(type?: string) {
	if (!type) return '';
	return type === 'single'
		? 'Single User'
		: type === 'multi'
			? 'Multi-User'
			: type === 'remote'
				? 'Remote'
				: 'Composite';
}

export function getServerType(server?: MCPCatalogServer): LaunchServerType | null {
	if (!server) return null;
	const runtime = server.manifest.runtime;
	if (runtime === 'remote') return 'remote';
	if (runtime === 'composite') return 'composite';
	return server.catalogEntryID ? 'single' : 'multi';
}

export function convertCompositeLaunchFormDataToPayload(lf: CompositeLaunchFormData) {
	const payload: Record<
		string,
		{ config: Record<string, string>; url?: string; disabled?: boolean }
	> = {};
	for (const [id, comp] of Object.entries(lf.componentConfigs)) {
		const config: Record<string, string> = {};
		for (const f of [
			...(comp.envs ?? ([] as Array<{ key: string; value: string }>)),
			...(comp.headers ?? ([] as Array<{ key: string; value: string }>))
		]) {
			if (f.value) config[f.key] = f.value;
		}
		payload[id] = {
			config,
			url: comp.url?.trim() || undefined,
			disabled: comp.disabled ?? false
		};
	}
	return payload;
}

export async function convertCompositeInfoToLaunchFormData(
	server: MCPCatalogServer,
	parent?: MCPCatalogEntry
) {
	let initial: Record<
		string,
		{ config: Record<string, string>; url?: string; disabled?: boolean }
	> = {};
	try {
		const revealed = await ChatService.revealCompositeMcpServer(server.id, {
			dontLogErrors: true
		});
		const rc = revealed as unknown as {
			componentConfigs?: Record<
				string,
				{ config: Record<string, string>; url?: string; disabled?: boolean }
			>;
		};
		initial = rc.componentConfigs ?? {};
	} catch (_error) {
		initial = {} as Record<
			string,
			{ config: Record<string, string>; url?: string; disabled?: boolean }
		>;
	}
	// Prefer existing server's runtime composite manifest for edit flows;
	// fall back to parent catalog entry only if server lacks composite config
	const components =
		server?.manifest?.compositeConfig?.componentServers ||
		(parent && 'manifest' in parent ? parent?.manifest?.compositeConfig?.componentServers : []) ||
		[];
	const componentConfigs: Record<
		string,
		{
			name?: string;
			icon?: string;
			hostname?: string;
			url?: string;
			disabled?: boolean;
			isMultiUser?: boolean;
			envs?: Array<Record<string, unknown> & { key: string; value: string }>;
			headers?: Array<Record<string, unknown> & { key: string; value: string }>;
		}
	> = {};
	for (const c of components) {
		const id = c.catalogEntryID || c.mcpServerID;
		if (!c.manifest || !id) continue;
		const m = c.manifest;
		const init = initial?.[id];
		// Treat components that reference an MCP server ID (and not a catalog
		// entry) as multi-user. Those are configured at the server level, so
		// per-user composite config should only allow enable/disable toggling.
		const isMultiUser = !!c.mcpServerID && !c.catalogEntryID;
		componentConfigs[id] = {
			name: m.name,
			icon: m.icon,
			hostname:
				isMultiUser || !(m.remoteConfig && 'hostname' in m.remoteConfig)
					? ''
					: m.remoteConfig.hostname,
			// For multi-user components, ignore any stored URL/config; they are
			// managed at the multi-user server level.
			url: isMultiUser ? undefined : (init?.url ?? m.remoteConfig?.fixedURL ?? ''),
			disabled: init?.disabled ?? false,
			isMultiUser,
			envs: isMultiUser
				? []
				: (m.env ?? []).map((e) => ({
						...(e as unknown as Record<string, unknown>),
						key: e.key,
						value: init?.config?.[e.key] ?? ''
					})),
			headers: isMultiUser
				? []
				: (m.remoteConfig?.headers ?? []).map((h) => ({
						...(h as unknown as Record<string, unknown>),
						key: h.key,
						value: init?.config?.[h.key] ?? ''
					}))
		};
	}
	return { componentConfigs } as CompositeLaunchFormData;
}

export function getServerUrl(d: MCPCatalogServer) {
	const belongsToWorkspace = d.powerUserWorkspaceID ? true : false;
	const isMulti = !d.catalogEntryID;

	let url = '';
	if (profile.current.hasAdminAccess?.()) {
		if (isMulti) {
			url = belongsToWorkspace
				? `/admin/mcp-servers/w/${d.powerUserWorkspaceID}/s/${d.id}/details`
				: `/admin/mcp-servers/s/${d.id}/details`;
		} else {
			url = belongsToWorkspace
				? `/admin/mcp-servers/w/${d.powerUserWorkspaceID}/c/${d.catalogEntryID}/instance/${d.id}/details`
				: `/admin/mcp-servers/c/${d.catalogEntryID}/instance/${d.id}/details`;
		}
	} else {
		url = isMulti
			? `/mcp-servers/s/${d.id}/details`
			: `/mcp-servers/c/${d.catalogEntryID}/instance/${d.id}/details`;
	}
	return url;
}

export const findServerAndEntryForProjectMcp = (mcpServer: ProjectMCP) => {
	const server = mcpServersAndEntries.current.userConfiguredServers.find(
		(s) => s.id === mcpServer.mcpID
	) || mcpServersAndEntries.current.servers.find((s) => s.id === mcpServer.mcpID);
	const entry = server?.catalogEntryID
		? mcpServersAndEntries.current.entries.find(
				(e) => e.id === server?.catalogEntryID
			)
		: undefined;
	return { server, entry };
};
import type { MCPCatalogEntry, MCPCatalogServer, MCPServerTool, ToolOverride } from '$lib/services';

export interface VMcpComponentView {
	key: string;
	name: string;
	icon?: string;
	description?: string;
	id?: string;
	toolOverrides?: ToolOverride[];
	toolPreview?: MCPServerTool[];
}

export function resolveVMcpComponents(
	vmcp: MCPCatalogEntry,
	entries: MCPCatalogEntry[],
	servers: MCPCatalogServer[]
): VMcpComponentView[] {
	return (vmcp.manifest.compositeConfig?.componentServers ?? []).map((component, index) => {
		const entry = entries.find((candidate) => candidate.id === component.catalogEntryID);
		const server = servers.find((candidate) => candidate.id === component.mcpServerID);
		const manifest = entry?.manifest ?? server?.manifest ?? component.manifest;
		const reference = component.catalogEntryID ?? component.mcpServerID;
		return {
			key: reference ?? `component-${index}`,
			name: manifest?.name || reference || 'Unknown server',
			icon: manifest?.icon,
			description: manifest?.shortDescription,
			id: reference,
			toolOverrides: component.toolOverrides,
			toolPreview: manifest?.toolPreview
		};
	});
}

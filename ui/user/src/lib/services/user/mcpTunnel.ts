import type { MCPCatalogEntry, MCPCatalogServer, TunnelConnection } from '$lib/services';

type TunnelAwareMCP = Pick<MCPCatalogEntry | MCPCatalogServer, 'manifest'>;

export function getMcpTunnelConnectionsKey(connections: TunnelConnection[] | undefined): string {
	if (connections === undefined) {
		return 'unknown';
	}

	return connections
		.map((connection) => connection.name)
		.sort()
		.join('\0');
}

export function isMcpTunnelDisconnected(
	item: TunnelAwareMCP | undefined,
	connections: TunnelConnection[] | undefined
): boolean {
	if (connections === undefined || item?.manifest.runtime !== 'remote') {
		return false;
	}

	const tunnelName = item.manifest.remoteConfig?.tunnelName;
	if (!tunnelName) {
		return false;
	}

	return !connections.some((connection) => connection.name === tunnelName);
}

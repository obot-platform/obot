import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
import { getMcpCatalogServer } from '../chat/operations';
import type { MCPCatalogServer } from '../chat/types';
import type { Fetcher } from '../http';
import AdminService from './index';

export async function getAdminCatalogMcpServer(
	id: string,
	opts?: { fetch?: Fetcher }
): Promise<MCPCatalogServer> {
	return AdminService.getMCPCatalogServer(DEFAULT_MCP_CATALOG_ID, id, opts);
}

export async function getAdminDirectMcpServer(
	id: string,
	opts?: { fetch?: Fetcher }
): Promise<MCPCatalogServer> {
	return getMcpCatalogServer(id, opts);
}

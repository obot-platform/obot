import type { Fetcher, Profile } from '$lib/services';
import { getMCPCatalogEntry, getMCPCatalogServer, getSingleOrRemoteMcpServer } from './utils';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const adminService = {
	getMCPCatalogEntry: vi.fn(),
	getMCPCatalogServer: vi.fn()
};

const userService = {
	getMCP: vi.fn(),
	getMcpCatalogServer: vi.fn(),
	getSingleOrRemoteMcpServer: vi.fn(),
	getWorkspaceMCPCatalogEntry: vi.fn(),
	getWorkspaceMCPCatalogServer: vi.fn(),
	getWorkspaceCatalogEntryServer: vi.fn()
};

vi.mock('$lib/services', () => ({
	get AdminService() {
		return adminService;
	},
	get UserService() {
		return userService;
	}
}));

const fetch = vi.fn() as unknown as Fetcher;
const admin = { hasAdminAccess: () => true } as Profile;
const user = { hasAdminAccess: () => false } as Profile;

function urlFor(search: string) {
	return new URL(`https://obot.example.com/mcp-servers/c/entry-1${search}`);
}

describe('mcp-servers route loaders', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	describe('navigating from the Connectors list', () => {
		const url = urlFor('?from=connectors');

		it('loads catalog entries through the permissive user endpoint for admins', () => {
			getMCPCatalogEntry('entry-1', url, admin, fetch);

			expect(userService.getMCP).toHaveBeenCalledWith('entry-1', { fetch });
			expect(adminService.getMCPCatalogEntry).not.toHaveBeenCalled();
			expect(userService.getWorkspaceMCPCatalogEntry).not.toHaveBeenCalled();
		});

		it('loads multi-user servers through the permissive user endpoint for admins', () => {
			getMCPCatalogServer('server-1', url, admin, fetch);

			expect(userService.getMcpCatalogServer).toHaveBeenCalledWith('server-1', { fetch });
			expect(adminService.getMCPCatalogServer).not.toHaveBeenCalled();
		});

		it('ignores a workspace id carried in the url', () => {
			getMCPCatalogEntry('entry-1', urlFor('?from=connectors&wid=ws-1'), admin, fetch);
			getSingleOrRemoteMcpServer(
				'server-1',
				'entry-1',
				urlFor('?from=connectors&wid=ws-1'),
				admin,
				fetch
			);

			expect(userService.getMCP).toHaveBeenCalledWith('entry-1', { fetch });
			expect(userService.getSingleOrRemoteMcpServer).toHaveBeenCalledWith('server-1', { fetch });
			expect(userService.getWorkspaceMCPCatalogEntry).not.toHaveBeenCalled();
			expect(userService.getWorkspaceCatalogEntryServer).not.toHaveBeenCalled();
		});
	});

	describe('navigating from the Entries table', () => {
		it('loads catalog entries through the admin endpoint', () => {
			getMCPCatalogEntry('entry-1', urlFor(''), admin, fetch);

			expect(adminService.getMCPCatalogEntry).toHaveBeenCalledWith('default', 'entry-1', { fetch });
			expect(userService.getMCP).not.toHaveBeenCalled();
		});

		it('loads workspace-owned entries through the workspace endpoint', () => {
			getMCPCatalogEntry('entry-1', urlFor('?wid=ws-1'), admin, fetch);

			expect(userService.getWorkspaceMCPCatalogEntry).toHaveBeenCalledWith('ws-1', 'entry-1', {
				fetch
			});
			expect(adminService.getMCPCatalogEntry).not.toHaveBeenCalled();
		});

		it('loads multi-user servers through the admin and workspace endpoints', () => {
			getMCPCatalogServer('server-1', urlFor(''), admin, fetch);
			getMCPCatalogServer('server-2', urlFor('?wid=ws-1'), admin, fetch);

			expect(adminService.getMCPCatalogServer).toHaveBeenCalledWith('default', 'server-1', {
				fetch
			});
			expect(userService.getWorkspaceMCPCatalogServer).toHaveBeenCalledWith('ws-1', 'server-2', {
				fetch
			});
		});

		it('loads workspace-owned instances through the workspace endpoint', () => {
			getSingleOrRemoteMcpServer('server-1', 'entry-1', urlFor('?wid=ws-1'), admin, fetch);

			expect(userService.getWorkspaceCatalogEntryServer).toHaveBeenCalledWith(
				'ws-1',
				'entry-1',
				'server-1',
				{ fetch }
			);
		});
	});

	it('always uses the user endpoints for non-admins', () => {
		getMCPCatalogEntry('entry-1', urlFor('?wid=ws-1'), user, fetch);
		getMCPCatalogServer('server-1', urlFor('?wid=ws-1'), user, fetch);

		expect(userService.getMCP).toHaveBeenCalledWith('entry-1', { fetch });
		expect(userService.getMcpCatalogServer).toHaveBeenCalledWith('server-1', { fetch });
		expect(adminService.getMCPCatalogEntry).not.toHaveBeenCalled();
		expect(adminService.getMCPCatalogServer).not.toHaveBeenCalled();
	});
});

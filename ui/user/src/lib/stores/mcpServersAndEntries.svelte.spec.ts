import {
	AdminService,
	UserService,
	type MCPCatalogEntry,
	type MCPCatalogServer,
	type Profile
} from '$lib/services';
import { mcpServersAndEntries, profile } from '$lib/stores';
import { beforeEach, describe, expect, it, vi } from 'vitest';

describe('mcpServersAndEntries', () => {
	beforeEach(() => {
		vi.restoreAllMocks();
		profile.current = {
			id: 'admin-user',
			username: 'admin-user',
			email: 'admin@example.com',
			iconURL: '',
			role: 0,
			effectiveRole: 0,
			groups: [],
			loaded: true,
			hasAdminAccess: () => true
		} as Profile;
		mcpServersAndEntries.current = {
			entries: [],
			servers: [],
			userInstances: [],
			userConfiguredServers: [],
			loading: false,
			lastFetched: null,
			isInitialized: false
		};
	});

	function mockMcpRequests() {
		const requests = {
			adminEntries: vi.spyOn(AdminService, 'listMCPCatalogEntries').mockResolvedValue([]),
			adminServers: vi.spyOn(AdminService, 'listMCPCatalogServers').mockResolvedValue([]),
			workspaceEntries: vi
				.spyOn(AdminService, 'listAllUserWorkspaceCatalogEntries')
				.mockResolvedValue([]),
			workspaceServers: vi
				.spyOn(AdminService, 'listAllUserWorkspaceMCPServers')
				.mockResolvedValue([]),
			configuredServers: vi
				.spyOn(UserService, 'listSingleOrRemoteMcpServers')
				.mockResolvedValue([]),
			userEntries: vi.spyOn(UserService, 'listMCPs').mockResolvedValue([]),
			userServers: vi.spyOn(UserService, 'listMCPCatalogServers').mockResolvedValue([]),
			instances: vi.spyOn(UserService, 'listMcpServerInstances').mockResolvedValue([])
		};
		return requests;
	}

	it('avoids admin-wide catalog entries for an admin on the MCP servers page', async () => {
		const requests = mockMcpRequests();

		await mcpServersAndEntries.fetchData({ forceRefresh: true, scope: 'user' });

		expect(requests.configuredServers).toHaveBeenCalledOnce();
		expect(requests.userEntries).toHaveBeenCalledOnce();
		expect(requests.userEntries).toHaveBeenCalledWith({ minimal: true });
		expect(requests.userServers).toHaveBeenCalledOnce();
		expect(requests.instances).toHaveBeenCalledOnce();
		expect(requests.adminEntries).not.toHaveBeenCalled();
		expect(requests.adminServers).toHaveBeenCalledOnce();
		expect(requests.workspaceEntries).not.toHaveBeenCalled();
		expect(requests.workspaceServers).not.toHaveBeenCalled();
	});

	it('includes admin-created servers on the MCP servers page', async () => {
		const requests = mockMcpRequests();
		requests.adminServers.mockResolvedValue([{ id: 'admin-created-server' } as MCPCatalogServer]);
		requests.userEntries.mockResolvedValue([{ id: 'accessible-entry' } as MCPCatalogEntry]);

		await mcpServersAndEntries.fetchData({ forceRefresh: true, scope: 'user' });

		expect(mcpServersAndEntries.current.entries).toEqual([
			expect.objectContaining({ id: 'accessible-entry', canConnect: true })
		]);
		expect(mcpServersAndEntries.current.userConfiguredServers).toEqual([
			expect.objectContaining({ id: 'admin-created-server', canConnect: false })
		]);
	});

	it('fetches user-scoped catalog servers once for a non-admin', async () => {
		const requests = mockMcpRequests();
		profile.current = {
			...profile.current,
			hasAdminAccess: () => false
		} as Profile;

		await mcpServersAndEntries.fetchData({ forceRefresh: true, scope: 'user' });

		expect(requests.userServers).toHaveBeenCalledOnce();
		expect(requests.adminServers).not.toHaveBeenCalled();
	});

	it('does not let an older admin fetch overwrite a newer user fetch', async () => {
		const requests = mockMcpRequests();
		let resolveAdminEntries: (entries: MCPCatalogEntry[]) => void;
		requests.adminEntries.mockReturnValue(
			new Promise((resolve) => {
				resolveAdminEntries = resolve;
			})
		);
		requests.userEntries
			.mockResolvedValueOnce([])
			.mockResolvedValueOnce([{ id: 'user-entry' } as MCPCatalogEntry]);

		const adminFetch = mcpServersAndEntries.fetchData({ forceRefresh: true, scope: 'admin' });
		await vi.waitFor(() => expect(requests.adminEntries).toHaveBeenCalledOnce());

		await mcpServersAndEntries.fetchData({ forceRefresh: true, scope: 'user' });
		expect(mcpServersAndEntries.current.entries).toEqual([
			expect.objectContaining({ id: 'user-entry' })
		]);

		resolveAdminEntries!([{ id: 'admin-entry' } as MCPCatalogEntry]);
		await adminFetch;
		expect(mcpServersAndEntries.current.entries).toEqual([
			expect.objectContaining({ id: 'user-entry' })
		]);
	});

	it('refreshes when switching from cached user scope to admin scope', async () => {
		const requests = mockMcpRequests();
		await mcpServersAndEntries.fetchData({ forceRefresh: true, scope: 'user' });

		await mcpServersAndEntries.fetchData({ scope: 'admin' });

		expect(requests.adminEntries).toHaveBeenCalledOnce();
		expect(requests.adminServers).toHaveBeenCalledTimes(2);
		expect(requests.workspaceEntries).toHaveBeenCalledOnce();
		expect(requests.workspaceServers).toHaveBeenCalledOnce();
	});
});

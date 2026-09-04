import { worker } from '../../../tests/mocks/worker';
import McpCompositeOauth from './McpCompositeOauth.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function createLegacyServerResponse(id: string) {
	return {
		id,
		alias: 'Composite MCP',
		manifest: {
			name: 'Composite MCP',
			compositeConfig: { componentServers: [] }
		}
	};
}

function createVMCPResponse(id: string) {
	return {
		id,
		displayName: 'Virtual MCP',
		created: '2026-09-04T00:00:00Z',
		components: [
			{
				mcpServerCatalogEntryID: 'vmcp-component-entry',
				catalogEntry: {
					manifest: {
						name: 'Virtual MCP Component',
						runtime: 'remote'
					}
				}
			}
		]
	};
}

function mockConsentApis(pending: Array<Record<string, string>> = []) {
	const legacyMcpServerGet = vi.fn();
	const vmcpGet = vi.fn();
	const oauthPendingGet = vi.fn();

	worker.use(
		http.get('/api/mcp-servers/:id', ({ params }) => {
			const id = String(params.id);
			legacyMcpServerGet(id);
			return HttpResponse.json(createLegacyServerResponse(id));
		}),
		http.get('/api/vmcps/:id', ({ params }) => {
			const id = String(params.id);
			vmcpGet(id);
			return HttpResponse.json(createVMCPResponse(id));
		}),
		http.get('/api/oauth/composite/:id', ({ params }) => {
			oauthPendingGet(String(params.id));
			return HttpResponse.json(pending);
		})
	);

	return { legacyMcpServerGet, oauthPendingGet, vmcpGet };
}

describe('McpCompositeOauth', () => {
	it('loads migrated metadata by canonical ID while authenticating the legacy connection', async () => {
		const { legacyMcpServerGet, oauthPendingGet, vmcpGet } = mockConsentApis();
		render(McpCompositeOauth, { compositeMcpId: 'ms1legacy', vmcpId: 'vmcp1migrated' });
		await expect.element(page.getByRole('heading', { name: 'Virtual MCP' })).toBeVisible();
		await vi.waitFor(() => expect(oauthPendingGet).toHaveBeenCalledWith('ms1legacy'));
		expect(vmcpGet).toHaveBeenCalledWith('vmcp1migrated');
		expect(legacyMcpServerGet).not.toHaveBeenCalled();
	});
	it('loads vMCP metadata from the vMCP endpoint for vmcp1 IDs', async () => {
		const id = 'vmcp1-consent-test';
		const icon = 'https://example.com/named-vmcp-component.svg';
		const { legacyMcpServerGet, oauthPendingGet, vmcpGet } = mockConsentApis([
			{
				mcpServerID: 'vmcp-component-server',
				catalogEntryID: 'vmcp-component-entry',
				name: 'Named vMCP Component',
				icon,
				authURL: 'https://example.com/oauth'
			}
		]);

		render(McpCompositeOauth, { compositeMcpId: id });

		await expect.element(page.getByText('Named vMCP Component', { exact: true })).toBeVisible();
		await expect
			.element(page.getByRole('img', { name: 'icon', exact: true }))
			.toHaveAttribute('src', icon);
		await vi.waitFor(() => expect(vmcpGet).toHaveBeenCalledWith(id));
		expect(legacyMcpServerGet).not.toHaveBeenCalled();
		expect(oauthPendingGet).toHaveBeenCalledWith(id);
	});

	it('falls back to the pending MCP server ID when its name is not set', async () => {
		const id = 'vmcp1-consent-fallback';
		const mcpServerID = 'component-server-without-name';
		const { legacyMcpServerGet, oauthPendingGet, vmcpGet } = mockConsentApis([
			{
				mcpServerID,
				catalogEntryID: 'unknown-component-entry',
				authURL: 'https://example.com/oauth'
			}
		]);

		render(McpCompositeOauth, { compositeMcpId: id });

		await expect.element(page.getByText(mcpServerID, { exact: true })).toBeVisible();
		await expect
			.element(page.getByRole('img', { name: 'icon', exact: true }))
			.not.toBeInTheDocument();
		await vi.waitFor(() => expect(vmcpGet).toHaveBeenCalledWith(id));
		expect(legacyMcpServerGet).not.toHaveBeenCalled();
		expect(oauthPendingGet).toHaveBeenCalledWith(id);
	});

	it('continues loading legacy MCP metadata for non-vMCP IDs', async () => {
		const id = 'composite-consent-test';
		const { legacyMcpServerGet, oauthPendingGet, vmcpGet } = mockConsentApis();

		render(McpCompositeOauth, { compositeMcpId: id });

		await expect.element(page.getByText('All services authenticated successfully!')).toBeVisible();
		await vi.waitFor(() => expect(legacyMcpServerGet).toHaveBeenCalledWith(id));
		expect(vmcpGet).not.toHaveBeenCalled();
		expect(oauthPendingGet).toHaveBeenCalledWith(id);
	});
});

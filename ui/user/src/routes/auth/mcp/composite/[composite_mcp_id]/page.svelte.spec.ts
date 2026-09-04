import { worker } from '../../../../../tests/mocks/worker';
import ConsentPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const oauthAuthRequestId = 'oar1fzwcp';

function mockConsentApis(pending: Array<Record<string, string>> = []) {
	const legacyMcpServerGet = vi.fn();
	const vmcpGet = vi.fn();
	const oauthPendingGet = vi.fn();

	worker.use(
		http.get('/api/mcp-servers/:id', ({ params }) => {
			const id = String(params.id);
			legacyMcpServerGet(id);
			return HttpResponse.json({
				id,
				alias: 'Composite MCP',
				manifest: {
					name: 'Composite MCP',
					compositeConfig: { componentServers: [] }
				}
			});
		}),
		http.get('/api/vmcps/:id', ({ params }) => {
			const id = String(params.id);
			vmcpGet(id);
			return HttpResponse.json({
				id,
				displayName: 'Virtual MCP',
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
			});
		}),
		http.get('/api/oauth/composite/:id', ({ params, request }) => {
			oauthPendingGet({
				id: String(params.id),
				oauthAuthRequestId: new URL(request.url).searchParams.get('oauth_auth_request')
			});
			return HttpResponse.json(pending);
		})
	);

	return { legacyMcpServerGet, oauthPendingGet, vmcpGet };
}

function renderConsentPage(compositeMcpId: string) {
	return render(ConsentPage, {
		data: {
			compositeMcpId,
			oauthAuthRequestId
		}
	});
}

describe('MCP OAuth consent route', () => {
	it('uses the vMCP endpoint for the vmcp1 consent URL', async () => {
		const id = 'vmcp1b6nlk';
		const { legacyMcpServerGet, oauthPendingGet, vmcpGet } = mockConsentApis([
			{
				mcpServerID: 'vmcp-component-server',
				catalogEntryID: 'vmcp-component-entry',
				name: 'Virtual MCP Component',
				authURL: 'https://example.com/oauth'
			}
		]);

		renderConsentPage(id);

		await expect.element(page.getByText('Virtual MCP Component', { exact: true })).toBeVisible();
		await vi.waitFor(() => expect(vmcpGet).toHaveBeenCalledWith(id));
		expect(legacyMcpServerGet).not.toHaveBeenCalled();
		expect(oauthPendingGet).toHaveBeenCalledWith({
			id,
			oauthAuthRequestId
		});
	});

	it('continues using the legacy endpoint for a non-vMCP consent URL', async () => {
		const id = 'composite-consent-test';
		const { legacyMcpServerGet, oauthPendingGet, vmcpGet } = mockConsentApis();

		renderConsentPage(id);

		await expect.element(page.getByText('Composite MCP', { exact: true })).toBeVisible();
		await vi.waitFor(() => expect(legacyMcpServerGet).toHaveBeenCalledWith(id));
		expect(vmcpGet).not.toHaveBeenCalled();
		expect(oauthPendingGet).toHaveBeenCalledWith({
			id,
			oauthAuthRequestId
		});
	});
});

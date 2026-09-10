import { worker } from '../../../tests/mocks/worker';
import McpCompositeOauth from './McpCompositeOauth.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

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
	const vmcpGet = vi.fn();
	const oauthPendingGet = vi.fn();

	worker.use(
		http.get('/api/vmcps/:id', ({ params }) => {
			const id = String(params.id);
			vmcpGet(id);
			return HttpResponse.json(createVMCPResponse(id));
		}),
		http.get('/api/oauth/vmcp/:id', ({ params }) => {
			oauthPendingGet(String(params.id));
			return HttpResponse.json(pending);
		})
	);

	return { oauthPendingGet, vmcpGet };
}

describe('McpCompositeOauth', () => {
	it('loads migrated metadata by canonical ID while authenticating the legacy connection', async () => {
		const { oauthPendingGet, vmcpGet } = mockConsentApis();
		render(McpCompositeOauth, { compositeMcpId: 'ms1legacy', vmcpId: 'vmcp1migrated' });
		await expect.element(page.getByRole('heading', { name: 'Virtual MCP' })).toBeVisible();
		await expect.element(page.getByText('All services authenticated successfully!')).toBeVisible();
		await vi.waitFor(() => expect(oauthPendingGet).toHaveBeenCalledWith('ms1legacy'));
		expect(vmcpGet).toHaveBeenCalledWith('vmcp1migrated');
	});
	it('loads vMCP metadata from the vMCP endpoint for vmcp1 IDs', async () => {
		const id = 'vmcp1-consent-test';
		const icon = 'https://example.com/named-vmcp-component.svg';
		const { oauthPendingGet, vmcpGet } = mockConsentApis([
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
		expect(oauthPendingGet).toHaveBeenCalledWith(id);
	});

	it('falls back to the pending MCP server ID when its name is not set', async () => {
		const id = 'vmcp1-consent-fallback';
		const mcpServerID = 'component-server-without-name';
		const { oauthPendingGet, vmcpGet } = mockConsentApis([
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
		expect(oauthPendingGet).toHaveBeenCalledWith(id);
	});

	it('retries loading pending authentication when the page becomes visible', async () => {
		const id = 'vmcp1-pending-retry';
		let calls = 0;
		mockConsentApis();
		worker.use(
			http.get('/api/oauth/vmcp/:id', () => {
				calls += 1;
				if (calls === 1) {
					return HttpResponse.json({ message: 'Unable to load authentication' }, { status: 500 });
				}
				return HttpResponse.json([
					{ mcpServerID: 'github', name: 'GitHub', authURL: 'https://example.com/github' }
				]);
			})
		);

		render(McpCompositeOauth, { compositeMcpId: id });
		await expect.element(page.getByText('Unable to load authentication')).toBeVisible();

		document.dispatchEvent(new Event('visibilitychange'));

		await expect.element(page.getByText('GitHub', { exact: true })).toBeVisible();
		expect(calls).toBe(2);
	});

	it('checks only the launched service and shows row-local feedback', async () => {
		const id = 'vmcp1-row-check';
		const componentCheck = vi.fn();
		let finishCheck: () => void;
		const { oauthPendingGet } = mockConsentApis([
			{ mcpServerID: 'github', name: 'GitHub', authURL: 'https://example.com/github' },
			{ mcpServerID: 'slack', name: 'Slack', authURL: 'https://example.com/slack' }
		]);
		worker.use(
			http.get('/api/oauth/vmcp/:id/components/:componentID', async ({ params }) => {
				componentCheck(String(params.componentID));
				await new Promise<void>((resolve) => (finishCheck = resolve));
				return HttpResponse.json({});
			})
		);

		render(McpCompositeOauth, { compositeMcpId: id });
		await page.getByRole('link', { name: 'Authenticate' }).first().click();
		document.dispatchEvent(new Event('visibilitychange'));

		await vi.waitFor(() => expect(componentCheck).toHaveBeenCalledWith('github'));
		await expect.element(page.getByText('Checking for valid authentication…')).toBeVisible();
		await expect.element(page.getByText('Slack', { exact: true })).toBeVisible();
		expect(componentCheck).not.toHaveBeenCalledWith('slack');
		expect(oauthPendingGet).toHaveBeenCalledTimes(1);
		document.dispatchEvent(new Event('visibilitychange'));
		expect(componentCheck).toHaveBeenCalledTimes(1);
		finishCheck!();
		await expect.element(page.getByText('GitHub', { exact: true })).not.toBeInTheDocument();
		await expect.element(page.getByText('Slack', { exact: true })).toBeVisible();
	});

	it('retains a failed row and lets the user retry', async () => {
		const id = 'vmcp1-row-retry';
		let calls = 0;
		const { oauthPendingGet } = mockConsentApis([
			{ mcpServerID: 'github', name: 'GitHub', authURL: 'https://example.com/github' }
		]);
		worker.use(
			http.get('/api/oauth/vmcp/:id/components/:componentID', () => {
				calls += 1;
				if (calls === 1) return HttpResponse.json({ authURL: 'https://example.com/new' });
				if (calls === 2)
					return HttpResponse.json({ message: 'Authentication check failed' }, { status: 500 });
				return HttpResponse.json({});
			})
		);

		const onComplete = vi.fn();
		render(McpCompositeOauth, { compositeMcpId: id, onComplete });
		await page.getByRole('link', { name: 'Authenticate' }).click();
		document.dispatchEvent(new Event('visibilitychange'));
		await vi.waitFor(() => expect(calls).toBe(1));
		await expect
			.element(page.getByRole('link', { name: 'Authenticate' }))
			.toHaveAttribute('href', 'https://example.com/new');

		await page.getByRole('link', { name: 'Authenticate' }).click();
		document.dispatchEvent(new Event('visibilitychange'));
		await vi.waitFor(() => expect(calls).toBe(2));
		await expect.element(page.getByText('Authentication check failed')).toBeVisible();

		await page.getByRole('button', { name: 'Retry' }).click();
		await vi.waitFor(() => expect(calls).toBe(3));

		await vi.waitFor(() => expect(onComplete).toHaveBeenCalledTimes(1));
		expect(oauthPendingGet).toHaveBeenCalledTimes(1);
	});
});

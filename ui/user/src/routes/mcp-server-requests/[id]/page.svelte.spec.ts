import { Group } from '$lib/services';
import type { MCPServerRequest } from '$lib/services/mcpserverrequests';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import type { PageData } from './$types';
import RequestPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock(import('$lib/url'), async (importOriginal) => ({
	...(await importOriginal()),
	goto: vi.fn()
}));

const pending: MCPServerRequest = {
	id: 'msrq1pending',
	created: new Date().toISOString(),
	name: 'Sentry',
	description: 'Query Sentry issues and events',
	reason: 'Debug production errors from my editor',
	sourceURL: 'https://docs.sentry.io/product/sentry-mcp/',
	runtime: 'remote',
	remoteURL: 'https://mcp.sentry.dev/mcp',
	config: [{ key: 'Authorization', description: 'Sentry token', sensitive: true }],
	suggestedVMCPID: 'vmcp1eng',
	requesterID: '3',
	requesterName: 'Dev User',
	state: 'pending'
};

const vmcps = [
	{ id: 'vmcp1ops', displayName: 'Operations', created: '' },
	{ id: 'vmcp1eng', displayName: 'Engineering', created: '' },
	{ id: 'vmcp1mine', displayName: 'Personal', created: '', userID: '1' }
];

function mockRequest(request: MCPServerRequest = pending) {
	worker.use(
		http.get('/api/mcp-server-requests/msrq1pending', () => HttpResponse.json(request)),
		http.get('/api/vmcps', () => HttpResponse.json({ items: vmcps })),
		http.get('/api/vmcps/:id', ({ params }) =>
			HttpResponse.json(vmcps.find((v) => v.id === params.id) ?? {})
		)
	);
}

async function renderPage(groups: string[] = [Group.ADMIN]) {
	localStorage.setItem('seenSplashDialog', new Date().toISOString());
	const data = await preparePageData<PageData>({ profile: createMockProfile(groups) });
	return render(RequestPage, {
		data: { ...data, id: 'msrq1pending' },
		params: { id: 'msrq1pending' }
	});
}

describe('Server request', () => {
	it('shows the request and suggests the requester’s vMCP', async () => {
		mockRequest();
		await renderPage();

		await expect.element(page.getByText('https://mcp.sentry.dev/mcp')).toBeInTheDocument();
		await expect
			.element(page.getByText('Debug production errors from my editor'))
			.toBeInTheDocument();
		await expect.element(page.getByText('Authorization')).toBeInTheDocument();
		const select = page.getByRole('combobox', { name: 'Choose a shared vMCP' });
		await expect.element(select).toHaveValue('vmcp1eng');
		// Personal vMCPs are not destinations.
		await expect.element(page.getByRole('option', { name: 'Personal' })).not.toBeInTheDocument();
	});

	it('approves into an existing vMCP', async () => {
		mockRequest();
		const approved = vi.fn();
		worker.use(
			http.post('/api/mcp-server-requests/msrq1pending/approve', async ({ request }) => {
				const body = await request.json();
				approved(body);
				return HttpResponse.json({
					...pending,
					state: 'approved',
					vmcpID: 'vmcp1eng',
					decidedAt: new Date().toISOString(),
					requesterCanConnect: false
				});
			})
		);
		await renderPage();

		await page.getByRole('textbox', { name: 'Note for the requester' }).fill('Welcome aboard');
		await page.getByRole('button', { name: 'Approve' }).click();

		await vi.waitFor(() =>
			expect(approved).toHaveBeenCalledWith({
				vmcpID: 'vmcp1eng',
				grantRequesterAccess: true,
				reason: 'Welcome aboard'
			})
		);
		await expect
			.element(page.getByText(/Added to the catalog and to Engineering/))
			.toBeInTheDocument();
		await expect
			.element(page.getByText(/The requester can't connect to this vMCP/))
			.toBeInTheDocument();
	});

	it('approves into a new vMCP named after the server by default', async () => {
		mockRequest();
		const approved = vi.fn();
		worker.use(
			http.post('/api/mcp-server-requests/msrq1pending/approve', async ({ request }) => {
				approved(await request.json());
				return HttpResponse.json({ ...pending, state: 'approved', vmcpID: 'vmcp1new' });
			})
		);
		await renderPage();

		await page.getByRole('radio', { name: 'A new vMCP' }).click();
		await expect.element(page.getByRole('textbox', { name: 'vMCP name' })).toHaveValue('Sentry');
		await page.getByRole('textbox', { name: 'vMCP description' }).fill('Observability');
		await page.getByRole('button', { name: 'Approve' }).click();

		await vi.waitFor(() =>
			expect(approved).toHaveBeenCalledWith({
				newVMCP: { displayName: 'Sentry', description: 'Observability' }
			})
		);
	});

	it('denies with a reason', async () => {
		mockRequest();
		const denied = vi.fn();
		worker.use(
			http.post('/api/mcp-server-requests/msrq1pending/deny', async ({ request }) => {
				denied(await request.json());
				return HttpResponse.json({
					...pending,
					state: 'denied',
					decisionReason: 'Use the existing Sentry vMCP',
					decidedAt: new Date().toISOString()
				});
			})
		);
		await renderPage();

		await page
			.getByRole('textbox', { name: 'Why are you denying it?' })
			.fill('Use the existing Sentry vMCP');
		await page.getByRole('button', { name: 'Deny' }).click();

		await vi.waitFor(() =>
			expect(denied).toHaveBeenCalledWith({ reason: 'Use the existing Sentry vMCP' })
		);
		await expect.element(page.getByText('Use the existing Sentry vMCP')).toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Approve' })).not.toBeInTheDocument();
	});

	it('shows requesters their request without decision controls', async () => {
		mockRequest();
		await renderPage([Group.USER]);

		await expect.element(page.getByText('https://mcp.sentry.dev/mcp')).toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Approve' })).not.toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Deny' })).not.toBeInTheDocument();
	});
});

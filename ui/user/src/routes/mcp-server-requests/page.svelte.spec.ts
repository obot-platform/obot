import { Group } from '$lib/services';
import type { MCPServerRequest } from '$lib/services/mcpserverrequests';
import { createMockProfile, preparePageData } from '../../tests/helpers/pageData';
import { worker } from '../../tests/mocks/worker';
import RequestsPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const pending: MCPServerRequest = {
	id: 'msrq1pending',
	created: new Date().toISOString(),
	name: 'Sentry',
	reason: 'Debug production errors from my editor',
	runtime: 'remote',
	remoteURL: 'https://mcp.sentry.dev/mcp',
	requesterID: '3',
	requesterName: 'Dev User',
	state: 'pending'
};

function mockRequests(requested = vi.fn()) {
	worker.use(
		http.get('/api/mcp-server-requests', ({ request }) => {
			const state = new URL(request.url).searchParams.get('state');
			requested(state);
			return HttpResponse.json({ items: state === 'approved' ? [] : [pending] });
		})
	);
	return requested;
}

async function renderPage(groups: string[] = [Group.ADMIN]) {
	localStorage.setItem('seenSplashDialog', new Date().toISOString());
	await preparePageData({ profile: createMockProfile(groups) });
	return render(RequestsPage);
}

describe('Server requests', () => {
	it('lists pending requests with their requester for administrators', async () => {
		const requested = mockRequests();
		await renderPage();

		await vi.waitFor(() => expect(requested).toHaveBeenCalledWith('pending'));
		const link = page.getByRole('link', { name: /Sentry/ });
		await expect.element(link).toHaveAttribute('href', '/mcp-server-requests/msrq1pending');
		await expect.element(link.getByText('https://mcp.sentry.dev/mcp')).toBeInTheDocument();
		await expect.element(link.getByText(/Requested by Dev User/)).toBeInTheDocument();
	});

	it('switches state filters', async () => {
		const requested = mockRequests();
		await renderPage();

		await page.getByRole('button', { name: 'Approved' }).click();
		await vi.waitFor(() => expect(requested).toHaveBeenCalledWith('approved'));
		await expect.element(page.getByText('No requests here.')).toBeInTheDocument();
	});

	it('shows administrators both built-in servers and the managed settings', async () => {
		mockRequests();
		await renderPage();

		const setup = page.getByRole('region', { name: 'Obot MCP server for your agents' });
		await expect.element(setup.getByText(/\/mcp-connect\/obot$/)).toBeInTheDocument();
		await expect.element(setup.getByText(/\/mcp-connect\/obot-admin$/)).toBeInTheDocument();
		await expect.element(setup.getByText('managed-mcp.json')).toBeInTheDocument();
		await expect.element(setup.getByText('Claude managed settings')).toBeInTheDocument();
	});

	it('shows basic users only their own server and no requester names', async () => {
		mockRequests();
		await renderPage([Group.USER]);

		const setup = page.getByRole('region', { name: 'Obot MCP server for your agents' });
		await expect.element(setup.getByText(/\/mcp-connect\/obot$/)).toBeInTheDocument();
		await expect.element(setup.getByText(/\/mcp-connect\/obot-admin$/)).not.toBeInTheDocument();
		await expect.element(setup.getByText('managed-mcp.json')).not.toBeInTheDocument();
		const link = page.getByRole('link', { name: /Sentry/ });
		await expect.element(link).toBeInTheDocument();
		await expect.element(link.getByText(/Requested by/)).not.toBeInTheDocument();
	});
});

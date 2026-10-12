import { worker } from '../../../../../tests/mocks/worker';
import Page from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

it('shows the scoped login command for the client authorization request', async () => {
	worker.use(
		http.get('*/oauth/mcp/login/:state', () => HttpResponse.json({})),
		http.get('*/oauth/mcp/login-request/original-request', () =>
			HttpResponse.json({
				authURL: `${window.location.origin}/oauth/mcp/login/ui-local-original`
			})
		)
	);
	await render(Page, { data: { authRequestID: 'original-request' } });
	await expect
		.element(page.getByRole('heading', { name: 'Authenticate with the MCP server' }))
		.toBeVisible();
	await expect
		.element(page.getByLabelText('Authentication command'))
		.toHaveTextContent(
			`obot mcp login --url '${window.location.origin}/oauth/mcp/login/ui-local-original'`
		);
});

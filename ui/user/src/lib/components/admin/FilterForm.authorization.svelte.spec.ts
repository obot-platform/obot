import type { MCPFilterInput } from '$lib/services';
import { preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import FilterForm from './FilterForm.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function filter(authorization = false): MCPFilterInput {
	return {
		id: 'authorization-test',
		name: 'CRM filter',
		url: 'https://content.example/filter',
		resources: [{ type: 'mcpCatalog', id: 'default' }],
		selectors: [{ method: 'tools/call', identifiers: ['get_customer'] }],
		...(authorization
			? {
					authorization: { enabled: true, url: 'https://policy.example/access/v1/evaluation' },
					hasAuthorizationSecret: true
				}
			: {})
	} as MCPFilterInput;
}
function mockSave() {
	const save = vi.fn();
	worker.use(
		http.post('/api/mcp-webhook-validations/authorization-test/reveal', () =>
			HttpResponse.json({})
		),
		http.put('/api/mcp-webhook-validations/authorization-test', async ({ request }) => {
			const body = await request.json();
			save(body);
			return HttpResponse.json({ ...(body as object), id: 'authorization-test' });
		}),
		http.post('/api/mcp-webhook-validations/authorization-test/launch', () =>
			HttpResponse.json({ success: true })
		)
	);
	return save;
}
describe('optional AuthZEN callback on custom webhook filters', () => {
	it('keeps existing filters unchanged', async () => {
		await preparePageData();
		const save = mockSave();
		render(FilterForm, { filter: filter() });
		await expect
			.element(page.getByRole('checkbox', { name: 'Authorize requests with AuthZEN' }))
			.not.toBeChecked();
		await expect
			.element(page.getByLabelText('Authorization endpoint', { exact: true }))
			.not.toBeInTheDocument();
		await page.getByRole('button', { name: 'Save', exact: true }).click();
		await vi.waitFor(() => expect(save).toHaveBeenCalled());
		expect(save.mock.calls[0][0].authorization).toBeUndefined();
		expect(save.mock.calls[0][0].selectors).toEqual([
			{ method: 'tools/call', identifiers: ['get_customer'] }
		]);
	});
	it('adds authorization without changing content settings or selectors', async () => {
		await preparePageData();
		const save = mockSave();
		render(FilterForm, { filter: filter() });
		await page.getByRole('checkbox', { name: 'Authorize requests with AuthZEN' }).click();
		await page
			.getByLabelText('Authorization endpoint', { exact: true })
			.fill('https://policy.example/access/v1/evaluation');
		await page
			.getByLabelText('Authorization signing secret (optional)', { exact: true })
			.fill('separate-secret');
		await page.getByRole('button', { name: 'Save', exact: true }).click();
		await vi.waitFor(() => expect(save).toHaveBeenCalled());
		expect(save).toHaveBeenCalledWith(
			expect.objectContaining({
				url: 'https://content.example/filter',
				selectors: [{ method: 'tools/call', identifiers: ['get_customer'] }],
				authorization: {
					enabled: true,
					url: 'https://policy.example/access/v1/evaluation',
					secret: 'separate-secret'
				}
			})
		);
	});
	it('preserves a stored secret', async () => {
		await preparePageData();
		const save = mockSave();
		render(FilterForm, { filter: filter(true) });
		await expect
			.element(page.getByLabelText('Authorization signing secret (optional)', { exact: true }))
			.toHaveValue('');
		await page.getByRole('button', { name: 'Save', exact: true }).click();
		await vi.waitFor(() => expect(save).toHaveBeenCalled());
		expect(save.mock.calls[0][0].authorization).toEqual({
			enabled: true,
			url: 'https://policy.example/access/v1/evaluation'
		});
	});
	it('sends an explicit disabled authorization object', async () => {
		await preparePageData();
		const save = mockSave();
		render(FilterForm, { filter: filter(true) });
		await page.getByRole('checkbox', { name: 'Authorize requests with AuthZEN' }).click();
		await page.getByRole('button', { name: 'Save', exact: true }).click();
		await vi.waitFor(() => expect(save).toHaveBeenCalled());
		expect(save.mock.calls[0][0].authorization.enabled).toBe(false);
	});
	it('requires a replacement secret when changing the endpoint', async () => {
		await preparePageData();
		const save = mockSave();
		render(FilterForm, { filter: filter(true) });
		await page
			.getByLabelText('Authorization endpoint', { exact: true })
			.fill('https://different.example/evaluate');
		await page.getByRole('button', { name: 'Save', exact: true }).click();
		await expect
			.element(
				page.getByText(
					'Enter an HTTPS endpoint. If signing is configured, re-enter the secret when changing the endpoint.'
				)
			)
			.toBeVisible();
		expect(save).not.toHaveBeenCalled();
	});
	it('enables authorization without a signing secret', async () => {
		await preparePageData();
		const save = mockSave();
		render(FilterForm, { filter: filter() });
		await page.getByRole('checkbox', { name: 'Authorize requests with AuthZEN' }).click();
		await page
			.getByLabelText('Authorization endpoint', { exact: true })
			.fill('https://policy.example/evaluate');
		await page.getByRole('button', { name: 'Save', exact: true }).click();
		await vi.waitFor(() => expect(save).toHaveBeenCalled());
		expect(save.mock.calls[0][0].authorization).toEqual({
			enabled: true,
			url: 'https://policy.example/evaluate'
		});
	});
});

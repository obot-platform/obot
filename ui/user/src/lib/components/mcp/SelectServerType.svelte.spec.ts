import { Group } from '$lib/services';
import { profile } from '$lib/stores';
import { createMockProfile } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import SelectServerType from './SelectServerType.svelte';
import { http, HttpResponse } from 'msw';
import { expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

it('offers hosted and remote servers, but not legacy composites', async () => {
	profile.initialize(createMockProfile());
	const onSelectServerType = vi.fn();
	const result = await render(SelectServerType, { onSelectServerType });
	result.component.open();

	await expect.element(page.getByRole('button', { name: /Hosted Server/ })).toBeVisible();
	await expect.element(page.getByRole('button', { name: /OpenAPI/ })).toBeVisible();
	await expect
		.element(page.getByRole('button', { name: /Composite Server/ }))
		.not.toBeInTheDocument();
	await page.getByRole('button', { name: /Remote Server/ }).click();
	expect(onSelectServerType).toHaveBeenCalledWith('remote');
});

it.each([
	{ role: 'owner', groups: [Group.OWNER, Group.ADMIN], allowed: true },
	{ role: 'admin', groups: [Group.ADMIN], allowed: true },
	{ role: 'power user', groups: [Group.POWERUSER], allowed: false },
	{ role: 'power user plus', groups: [Group.POWERUSER_PLUS, Group.POWERUSER], allowed: false },
	{ role: 'auditor', groups: [Group.AUDITOR], allowed: false },
	{ role: 'basic user', groups: [], allowed: false }
])('offers OpenAPI to $role only when import is authorized', async ({ groups, allowed }) => {
	profile.initialize(createMockProfile(groups));
	const onSelectServerType = vi.fn();
	const { component } = await render(SelectServerType, { onSelectServerType });
	component.open();
	const openAPI = page.getByRole('button', { name: /OpenAPI/ });
	if (allowed) {
		await expect.element(openAPI).toBeVisible();
		await openAPI.click();
		await expect
			.element(page.getByRole('heading', { name: 'Import OpenAPI schema' }))
			.toBeVisible();
		expect(onSelectServerType).not.toHaveBeenCalled();
	} else {
		await expect.element(openAPI).not.toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: /Hosted Server/ })).toBeVisible();
		await expect.element(page.getByRole('button', { name: /Remote Server/ })).toBeVisible();
		expect(onSelectServerType).not.toHaveBeenCalled();
	}
});

it('returns to the type picker when OpenAPI import is cancelled', async () => {
	profile.initialize(createMockProfile([Group.ADMIN]));
	const onSelectServerType = vi.fn();
	const { component } = await render(SelectServerType, { onSelectServerType });
	component.open();
	await page.getByRole('button', { name: /OpenAPI/ }).click();
	await page.getByRole('button', { name: 'Cancel', exact: true }).click();
	await expect.element(page.getByRole('heading', { name: 'Select Server Type' })).toBeVisible();
	expect(onSelectServerType).not.toHaveBeenCalled();
});

it('selects OpenAPI only after a successful import', async () => {
	const result = {
		baseURL: '',
		destinationValidation: { valid: false, error: 'no usable server URL; configure baseURL' },
		suggestedHeaders: [],
		suggestedMetadata: { name: 'Example' }
	};
	worker.use(http.post('/api/openapi-import', () => HttpResponse.json(result)));
	profile.initialize(createMockProfile([Group.ADMIN]));
	const onSelectServerType = vi.fn();
	const { component } = await render(SelectServerType, { onSelectServerType });
	component.open();
	await page.getByRole('button', { name: /OpenAPI/ }).click();
	await page.getByLabelText('Schema URL', { exact: true }).fill('https://example.com/schema.json');
	await page.getByRole('button', { name: 'Import', exact: true }).click();
	await vi.waitFor(() =>
		expect(onSelectServerType).toHaveBeenCalledWith('openapi', {
			config: expect.objectContaining({
				source: { url: 'https://example.com/schema.json' }
			}),
			result
		})
	);
});

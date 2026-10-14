import { Group } from '$lib/services';
import { profile } from '$lib/stores';
import { createMockProfile } from '../../../tests/helpers/pageData';
import SelectServerType from './SelectServerType.svelte';
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
		expect(onSelectServerType).toHaveBeenCalledWith('openapi');
	} else {
		await expect.element(openAPI).not.toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: /Hosted Server/ })).toBeVisible();
		await expect.element(page.getByRole('button', { name: /Remote Server/ })).toBeVisible();
		expect(onSelectServerType).not.toHaveBeenCalled();
	}
});

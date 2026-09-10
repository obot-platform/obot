import type { VMCPInstance } from '$lib/services';
import { vmcpInstances } from '$lib/stores';
import { preparePageData } from '../../../tests/helpers/pageData';
import { getProfileResponse } from '../../../tests/mocks/data';
import { worker } from '../../../tests/mocks/worker';
import VMcpCard from './VMcpCard.svelte';
import { http, HttpResponse } from 'msw';
import { createRawSnippet } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const icon = createRawSnippet(() => ({
	render: () => '<span></span>'
}));

const instance: VMCPInstance = {
	id: 'vmcpi-1',
	vmcpID: 'vmcp-1',
	userID: getProfileResponse.id,
	created: '2026-01-01T00:00:00Z'
};

async function renderCard() {
	await preparePageData();
	return render(VMcpCard, {
		id: 'vmcp-1',
		name: 'Issue Gateway',
		selectAriaLabel: 'Click to edit Issue Gateway',
		icon
	});
}

async function openActions() {
	await page.getByRole('button', { name: 'Actions for Issue Gateway' }).click();
}

describe('VMcpCard.svelte', () => {
	beforeEach(() => {
		vmcpInstances.current = { items: [], loading: false };
	});

	it('omits Disconnect when the user has no instance', async () => {
		await renderCard();
		await openActions();

		await expect.element(page.getByRole('button', { name: 'Disconnect' })).not.toBeInTheDocument();
	});

	it('deletes the user instance and removes it from the store', async () => {
		const deleted = vi.fn();
		worker.use(
			http.delete('/api/vmcp-instances/vmcpi-1', () => {
				deleted();
				return HttpResponse.json({});
			})
		);

		await preparePageData();
		vmcpInstances.current = { items: [instance], loading: false };
		render(VMcpCard, {
			id: 'vmcp-1',
			name: 'Issue Gateway',
			selectAriaLabel: 'Click to edit Issue Gateway',
			icon
		});
		await openActions();
		await page.getByRole('button', { name: 'Disconnect' }).click();

		await vi.waitFor(() => expect(vmcpInstances.current.items).toEqual([]));
		expect(deleted).toHaveBeenCalledOnce();
		await expect.element(page.getByRole('button', { name: 'Disconnect' })).not.toBeInTheDocument();
	});
});

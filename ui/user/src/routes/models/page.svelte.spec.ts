import { page as appPage } from '$app/state';
import type { MessagePolicy } from '$lib/services';
import { preparePageData } from '../../tests/helpers/pageData';
import { getVersionResponse } from '../../tests/mocks/data';
import type { PageData } from './$types';
import ModelsPage from './+page.svelte';
import { afterEach, describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const toolPolicy: MessagePolicy = {
	id: 'tool-policy',
	displayName: 'Block shell tools',
	definition: 'Do not allow shell tools',
	direction: 'tool-calls',
	created: '2026-01-01T00:00:00Z',
	subjects: []
};
const userPolicy: MessagePolicy = {
	id: 'user-policy',
	displayName: 'Block travel booking',
	definition: 'Do not allow travel booking',
	direction: 'user-message',
	created: '2026-01-01T00:00:00Z',
	subjects: []
};

async function renderModelsPage({
	view = 'models',
	messagePolicies = [],
	messagePoliciesEnabled = false
}: {
	view?: string;
	messagePolicies?: MessagePolicy[];
	messagePoliciesEnabled?: boolean;
} = {}) {
	appPage.url.searchParams.set('view', view);
	localStorage.setItem('seenSplashDialog', new Date().toISOString());
	const data = await preparePageData<PageData>({
		models: [],
		modelProviders: [],
		modelAccessPolicies: [],
		messagePolicies,
		hasAdminAccess: true,
		...(messagePoliciesEnabled
			? { version: { ...getVersionResponse, messagePoliciesEnabled: true } }
			: {})
	});
	return render(ModelsPage, { data });
}

afterEach(() => {
	appPage.url.searchParams.delete('view');
	appPage.url.searchParams.delete('new');
});

describe('Models page message policies tab', () => {
	it('shows user-message policies and hides tool-call policies', async () => {
		await renderModelsPage({
			view: 'ai-judge-policies',
			messagePoliciesEnabled: true,
			messagePolicies: [toolPolicy, userPolicy]
		});

		await expect.element(page.getByRole('button', { name: 'AI Judge Policies' })).toBeVisible();
		await expect.element(page.getByRole('row', { name: /Block travel booking/ })).toBeVisible();
		await expect
			.element(page.getByRole('row', { name: /Block shell tools/ }))
			.not.toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Add AI Judge Policy' })).toBeVisible();
	});

	it('hides the tab when message policies are disabled', async () => {
		await renderModelsPage({ view: 'access-policies' });

		await expect
			.element(page.getByRole('button', { name: 'AI Judge Policies' }))
			.not.toBeInTheDocument();
	});
});

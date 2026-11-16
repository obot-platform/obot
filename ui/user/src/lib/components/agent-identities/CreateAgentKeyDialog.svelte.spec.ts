import { renderOpenDialog } from '../../../tests/helpers/openDialog';
import { preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import CreateAgentKeyDialog from './CreateAgentKeyDialog.svelte';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';

const submitted = vi.fn();

beforeEach(async () => {
	submitted.mockReset();
	worker.use(
		http.post('/api/agents/7/api-keys', async ({ request }) => {
			submitted(await request.json());
			return HttpResponse.json({ id: 1, agentId: '7', key: 'oak1-secret' }, { status: 201 });
		})
	);
	await preparePageData();
});

async function renderDialog() {
	await renderOpenDialog(CreateAgentKeyDialog, { agentId: '7', onCreated: vi.fn() });
}

describe('CreateAgentKeyDialog', () => {
	it('creates a key that can use everything the agent can', async () => {
		await renderDialog();
		await page.getByRole('textbox', { name: 'Name', exact: true }).fill('ci key');
		await page.getByRole('button', { name: 'Create', exact: true }).click();

		await expect
			.poll(() => submitted.mock.calls[0]?.[0])
			.toEqual({
				name: 'ci key',
				canAccessLLMProxy: true,
				canAccessSkills: true,
				mcpServerIds: ['*']
			});
	});

	it('offers no scope choices', async () => {
		await renderDialog();

		await expect
			.element(page.getByText(/This key can use everything the agent has access to/))
			.toBeVisible();
		await expect.element(page.getByRole('radio')).not.toBeInTheDocument();
		await expect.element(page.getByRole('checkbox')).not.toBeInTheDocument();
	});
});

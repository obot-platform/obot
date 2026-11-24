import { HttpError } from '$lib/errors';
import { Group } from '$lib/services';
import type { AgentProfile } from '$lib/services/agent-identities/types';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import { createAgent } from '../../../tests/mocks/agentIdentities';
import { worker } from '../../../tests/mocks/worker';
import AgentProfilesTab from './AgentProfilesTab.svelte';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const ada = { id: 'u1', displayName: 'Ada Admin', status: 'active' };
const entries: AgentProfile[] = [
	{
		id: 'ap1all',
		name: 'Everyone',
		subjects: [{ type: 'selector', id: '*' }]
	},
	{
		id: 'ap1some',
		name: 'Analysts',
		subjects: [{ type: 'user', id: 'u1' }]
	}
];

const onSave = vi.fn();

beforeEach(async () => {
	onSave.mockReset();
	onSave.mockResolvedValue(undefined);
	worker.use(
		http.get('*/api/users', () => HttpResponse.json({ items: [ada] })),
		http.get('*/api/groups', () => HttpResponse.json({ items: [] })),
		http.get('*/api/agents', () =>
			HttpResponse.json({
				items: [{ id: '5', displayName: 'Build bot', status: 'active', activeAPIKeyCount: 1 }]
			})
		)
	);
	await preparePageData({ profile: createMockProfile([Group.ADMIN]) });
});

function renderTab(props: Record<string, unknown> = {}) {
	return render(AgentProfilesTab, {
		agent: createAgent({ profiles: entries, allowedReturnHosts: ['agent.example.com'] }),
		onSave,
		...props
	});
}

// The text of a row, whose cells have no whitespace between them.
const textOf = (locator: { element(): Element }) =>
	expect.poll(() => locator.element().textContent ?? '');

async function fillNewEntry(name: string) {
	await page.getByRole('button', { name: 'Add User Access' }).click();
	await page.getByLabelText('Name', { exact: true }).fill(name);
	await page.getByRole('button', { name: 'Add User/Group' }).click();
	await page.getByRole('button', { name: /Ada Admin/ }).click();
	await page.getByRole('button', { name: 'Confirm' }).click();
}

describe('AgentProfilesTab: the list', () => {
	it('lists each entry with the users and groups it matches', async () => {
		renderTab();

		const everyone = page.getByRole('row', { name: /Everyone/ });
		await expect.element(everyone).toBeVisible();
		await textOf(everyone).toContain('All Obot Users');

		const analysts = page.getByRole('row', { name: /Analysts/ });
		await textOf(analysts).toContain('Ada Admin');
	});

	it('deletes an entry after a confirmation, sending the other entries back', async () => {
		renderTab();

		await page
			.getByRole('row', { name: /Analysts/ })
			.getByRole('button', { name: 'Delete' })
			.click();
		await expect.element(page.getByText('Delete the user access "Analysts"?')).toBeVisible();
		await page.getByRole('button', { name: "Yes, I'm sure" }).click();

		await vi.waitFor(() => expect(onSave).toHaveBeenCalled());
		expect(onSave.mock.calls[0][0]).toEqual({ profiles: [entries[0]] });
	});

	it("shows the server's error when a delete fails", async () => {
		onSave.mockRejectedValue(
			new HttpError(409, '409 /agents/7: the agent was changed by someone else, reload it')
		);
		renderTab();

		await page
			.getByRole('row', { name: /Analysts/ })
			.getByRole('button', { name: 'Delete' })
			.click();
		await page.getByRole('button', { name: "Yes, I'm sure" }).click();

		await expect
			.element(page.getByRole('alert'))
			.toHaveTextContent('the agent was changed by someone else, reload it');
		await expect.element(page.getByRole('row', { name: /Analysts/ })).toBeVisible();
	});

	it('hides the changes from read-only admins and auditors', async () => {
		renderTab({ readonly: true });

		await expect.element(page.getByRole('row', { name: /Analysts/ })).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Add User Access' }))
			.not.toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Edit' })).not.toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: 'Delete', exact: true }))
			.not.toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Save Hosts' })).not.toBeInTheDocument();
	});
});

describe('AgentProfilesTab: the form', () => {
	it('saves a new entry with its name and the users and groups it matches', async () => {
		renderTab({ agent: createAgent() });

		await fillNewEntry('Everyone');
		await page.getByRole('button', { name: 'Save', exact: true }).click();

		await vi.waitFor(() => expect(onSave).toHaveBeenCalled());
		const [{ profiles }] = onSave.mock.calls[0];
		expect(profiles).toEqual([{ name: 'Everyone', subjects: [{ type: 'user', id: 'u1' }] }]);
	});

	it('edits an entry in place, keeping its ID', async () => {
		renderTab();

		await page
			.getByRole('row', { name: /Analysts/ })
			.getByRole('button', { name: 'Edit' })
			.click();
		await page.getByLabelText('Name', { exact: true }).fill('Senior analysts');
		await page.getByRole('button', { name: 'Save', exact: true }).click();

		await vi.waitFor(() => expect(onSave).toHaveBeenCalled());
		const { profiles } = onSave.mock.calls[0][0];
		expect(profiles).toHaveLength(2);
		expect(profiles[1]).toEqual({ ...entries[1], name: 'Senior analysts' });
	});

	it('does not offer agents as subjects', async () => {
		renderTab({ agent: createAgent() });

		await page.getByRole('button', { name: 'Add User Access' }).click();
		await page.getByRole('button', { name: 'Add User/Group' }).click();

		await expect.element(page.getByRole('button', { name: /Ada Admin/ })).toBeVisible();
		await expect.element(page.getByRole('button', { name: /All Obot Users/ })).toBeVisible();
		await expect.element(page.getByText('Build bot')).not.toBeInTheDocument();
	});

	it('asks for a name and a subject before saving', async () => {
		renderTab({ agent: createAgent() });

		await page.getByRole('button', { name: 'Add User Access' }).click();
		await page.getByRole('button', { name: 'Save', exact: true }).click();

		await expect.element(page.getByText('Name is required')).toBeVisible();
		await expect.element(page.getByText('Add at least one user or group.')).toBeVisible();
		expect(onSave).not.toHaveBeenCalled();
	});

	it('shows a server error next to the field it names', async () => {
		onSave.mockRejectedValue(
			new HttpError(
				400,
				'400 /agents/7: invalid profiles: agent profile "Mixed": invalid subject: agent subjects are not supported in agent profiles'
			)
		);
		renderTab({ agent: createAgent() });

		await fillNewEntry('Mixed');
		await page.getByRole('button', { name: 'Save', exact: true }).click();

		await expect
			.element(page.getByRole('alert'))
			.toHaveTextContent('invalid subject: agent subjects are not supported in agent profiles');
	});
});

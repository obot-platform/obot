import { page as appPage } from '$app/state';
import { Group, type MessagePolicy, type PolicyDirection } from '$lib/services';
import { openUrl } from '$lib/utils';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import MessagePoliciesView from './MessagePoliciesView.svelte';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock('$lib/utils', async (importOriginal) => ({
	...(await importOriginal<typeof import('$lib/utils')>()),
	openUrl: vi.fn()
}));

function policy(id: string, displayName: string, direction: PolicyDirection): MessagePolicy {
	return {
		id,
		displayName,
		definition: 'rule',
		direction,
		created: '2026-01-01T00:00:00Z',
		subjects: []
	};
}

const policies = [
	policy('tool', 'Block shell tools', 'tool-calls'),
	policy('user', 'Block travel booking', 'user-message'),
	policy('both', 'Block everything', 'both')
];

async function renderView(
	props: {
		policyDirection?: 'tool-calls' | 'user-message';
		creating?: boolean;
		messagePolicies?: MessagePolicy[];
		contents?: 'policies' | 'policy-violations';
	} = {},
	groups: string[] = [Group.ADMIN]
) {
	if (props.contents) {
		appPage.url.searchParams.set('contents', props.contents);
	} else {
		appPage.url.searchParams.delete('contents');
	}
	await preparePageData({ profile: createMockProfile(groups) });
	return render(MessagePoliciesView, {
		messagePolicies: props.messagePolicies ?? policies,
		policyDirection: props.policyDirection,
		creating: props.creating
	});
}

function mockViolationApis() {
	worker.use(
		http.get('/api/message-policy-violations', ({ request }) => {
			const direction = new URL(request.url).searchParams.get('direction');
			const toolCall = direction === 'tool-calls';
			return HttpResponse.json({
				items: [
					{
						id: 1,
						createdAt: '2026-01-02T00:00:00Z',
						userID: 'user-1',
						policyID: toolCall ? 'tool' : 'user',
						policyName: toolCall ? 'Block shell tools' : 'Block travel booking',
						policyDefinition: 'rule',
						direction: toolCall ? 'tool-calls' : 'user-message',
						violationExplanation: 'blocked',
						projectID: '',
						threadID: ''
					}
				],
				total: 1
			});
		}),
		http.get('/api/message-policy-violation-stats', () =>
			HttpResponse.json({
				byTime: [],
				byPolicy: [],
				byUser: [],
				byDirection: { userMessage: 0, toolCalls: 1 }
			})
		),
		http.get('/api/message-policy-violations/filter-options/:filter', () =>
			HttpResponse.json({ options: [] })
		)
	);
}

function convertDialog() {
	return page.getByRole('dialog').filter({ hasText: 'Update Policy Direction' });
}

async function chooseDirection(label: string) {
	await convertDialog().getByRole('combobox', { name: 'Direction' }).click();
	await page.getByRole('button', { name: label, exact: true }).click();
}

async function clickPolicy(name: string) {
	await page.getByRole('row').filter({ hasText: name }).getByRole('cell').first().click();
}

beforeEach(() => {
	vi.mocked(openUrl).mockReset();
});

describe('MessagePoliciesView', () => {
	it('shows tool-call policies and preexisting both-direction policies for MCP servers', async () => {
		await renderView({ policyDirection: 'tool-calls' });

		await expect.element(page.getByRole('row', { name: /Block shell tools/ })).toBeVisible();
		await expect.element(page.getByRole('row', { name: /Block everything/ })).toBeVisible();
		await expect
			.element(page.getByRole('row', { name: /Block travel booking/ }))
			.not.toBeInTheDocument();
	});

	it('shows user-message policies and preexisting both-direction policies for models', async () => {
		await renderView({ policyDirection: 'user-message' });

		await expect.element(page.getByRole('row', { name: /Block travel booking/ })).toBeVisible();
		await expect.element(page.getByRole('row', { name: /Block everything/ })).toBeVisible();
		await expect
			.element(page.getByRole('row', { name: /Block shell tools/ }))
			.not.toBeInTheDocument();
	});

	it('shows every policy when no direction is set', async () => {
		await renderView();

		await expect.element(page.getByRole('row', { name: /Block shell tools/ })).toBeVisible();
		await expect.element(page.getByRole('row', { name: /Block travel booking/ })).toBeVisible();
		await expect.element(page.getByRole('row', { name: /Block everything/ })).toBeVisible();
	});

	it('shows an empty state when the filtered list is empty', async () => {
		await renderView({ policyDirection: 'tool-calls', messagePolicies: [] });

		await expect.element(page.getByRole('heading', { name: 'No message policies' })).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Add Message Policy' })).toBeVisible();
	});

	it('hides create actions for read-only admins', async () => {
		await renderView({ policyDirection: 'user-message', messagePolicies: [] }, [Group.AUDITOR]);

		await expect.element(page.getByRole('heading', { name: 'No message policies' })).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Add Message Policy' }))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByText('Click the button below to get started.'))
			.not.toBeInTheDocument();
	});

	it('opens the create form for a tool-call policy', async () => {
		await renderView({ policyDirection: 'tool-calls', creating: true, messagePolicies: [] });

		await expect.element(page.getByRole('textbox', { name: 'Name' })).toBeVisible();
	});

	it('opens the create form for a user-message policy', async () => {
		await renderView({ policyDirection: 'user-message', creating: true, messagePolicies: [] });

		await expect.element(page.getByRole('textbox', { name: 'Name' })).toBeVisible();
	});

	it('limits violation logs to tool calls', async () => {
		mockViolationApis();
		await renderView({ policyDirection: 'tool-calls', contents: 'policy-violations' });

		await expect.element(page.getByRole('row', { name: /Block shell tools/ })).toBeVisible();
		await expect
			.element(page.getByRole('row', { name: /Block travel booking/ }))
			.not.toBeInTheDocument();
		await expect.element(page.getByText('Tool Call Violations')).toBeVisible();
		await expect.element(page.getByText('User Message Violations')).not.toBeInTheDocument();
		await expect.element(page.getByRole('combobox', { name: /Filter by user/ })).toBeVisible();
	});

	it('limits violation logs to user messages', async () => {
		mockViolationApis();
		await renderView({ policyDirection: 'user-message', contents: 'policy-violations' });

		await expect.element(page.getByRole('row', { name: /Block travel booking/ })).toBeVisible();
		await expect
			.element(page.getByRole('row', { name: /Block shell tools/ }))
			.not.toBeInTheDocument();
		await expect.element(page.getByText('User Message Violations')).toBeVisible();
		await expect.element(page.getByText('Tool Call Violations')).not.toBeInTheDocument();
		await expect.element(page.getByRole('combobox', { name: /Filter by user/ })).toBeVisible();
	});

	it('opens a single-direction policy without asking to convert it', async () => {
		await renderView({ policyDirection: 'tool-calls' });

		await clickPolicy('Block shell tools');

		expect(openUrl).toHaveBeenCalledWith('/mcp-servers/message-policies/tool', false);
		await expect.element(convertDialog()).not.toBeInTheDocument();
	});

	it('asks how to handle a both-direction policy before opening it', async () => {
		await renderView({ policyDirection: 'tool-calls' });

		await clickPolicy('Block everything');

		await expect.element(convertDialog()).toBeVisible();
		await expect
			.element(
				convertDialog()
					.getByRole('combobox', { name: 'Direction' })
					.filter({ hasText: 'Tool Calls' })
			)
			.toBeVisible();
		await expect.element(convertDialog().getByRole('button', { name: 'Save' })).toBeVisible();
		await expect
			.element(convertDialog().getByRole('button', { name: 'Delete Policy' }))
			.toBeVisible();
		expect(openUrl).not.toHaveBeenCalled();
	});

	it('updates the direction to user messages before opening the policy', async () => {
		const calls: string[] = [];
		let body: unknown;
		vi.mocked(openUrl).mockImplementation(() => {
			calls.push('open');
		});
		worker.use(
			http.put('/api/message-policies/both', async ({ request }) => {
				calls.push('put');
				body = await request.json();
				return HttpResponse.json({ ...policies[2], direction: 'user-message' });
			})
		);
		await renderView({ policyDirection: 'tool-calls' });

		await clickPolicy('Block everything');
		await chooseDirection('User Messages');
		await convertDialog().getByRole('button', { name: 'Save' }).click();

		await vi.waitFor(() => {
			expect(calls).toEqual(['put', 'open']);
		});
		expect(body).toMatchObject({
			displayName: 'Block everything',
			definition: 'rule',
			direction: 'user-message'
		});
		expect(openUrl).toHaveBeenCalledWith('/models/message-policies/both', false);
	});

	it('updates the direction to tool calls before opening the policy', async () => {
		const calls: string[] = [];
		vi.mocked(openUrl).mockImplementation(() => {
			calls.push('open');
		});
		worker.use(
			http.put('/api/message-policies/both', async () => {
				calls.push('put');
				return HttpResponse.json({ ...policies[2], direction: 'tool-calls' });
			})
		);
		await renderView({ policyDirection: 'user-message' });

		await clickPolicy('Block everything');
		await chooseDirection('Tool Calls');
		await convertDialog().getByRole('button', { name: 'Save' }).click();

		await vi.waitFor(() => {
			expect(calls).toEqual(['put', 'open']);
		});
		expect(openUrl).toHaveBeenCalledWith('/mcp-servers/message-policies/both', false);
	});

	it('deletes a both-direction policy from the dialog without opening it', async () => {
		worker.use(
			http.delete('/api/message-policies/both', () => new HttpResponse(null, { status: 204 }))
		);
		await renderView({ policyDirection: 'tool-calls' });

		await clickPolicy('Block everything');
		await convertDialog().getByRole('button', { name: 'Delete Policy' }).click();

		await expect.element(convertDialog()).not.toBeInTheDocument();
		await expect.element(page.getByRole('row', { name: /Block everything/ })).toBeVisible();
		await page.getByRole('button', { name: "Yes, I'm sure", exact: true }).click();

		await expect
			.element(page.getByRole('row', { name: /Block everything/ }))
			.not.toBeInTheDocument();
		expect(openUrl).not.toHaveBeenCalled();
	});

	it('lets a read-only admin open a both-direction policy without converting it', async () => {
		await renderView({ policyDirection: 'tool-calls' }, [Group.AUDITOR]);

		await clickPolicy('Block everything');

		expect(openUrl).toHaveBeenCalledWith('/mcp-servers/message-policies/both', false);
		await expect.element(convertDialog()).not.toBeInTheDocument();
	});
});

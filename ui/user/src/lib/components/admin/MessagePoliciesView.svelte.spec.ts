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
		policyDirection: 'tool-calls' | 'user-message';
		creating?: boolean;
		messagePolicies?: MessagePolicy[];
		contents?: 'policies' | 'policy-violations';
	} = {
		policyDirection: 'tool-calls'
	},
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

	it('shows an empty state when the filtered list is empty', async () => {
		await renderView({ policyDirection: 'tool-calls', messagePolicies: [] });

		await expect.element(page.getByRole('heading', { name: 'No AI judge policies' })).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Add AI Judge Policy' })).toBeVisible();
	});

	it('hides create actions for read-only admins', async () => {
		await renderView({ policyDirection: 'user-message', messagePolicies: [] }, [Group.AUDITOR]);

		await expect.element(page.getByRole('heading', { name: 'No AI judge policies' })).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Add AI Judge Policy' }))
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
		await expect.element(page.getByText('Tool Call Violations')).not.toBeInTheDocument();
		await expect.element(page.getByRole('combobox', { name: /Filter by user/ })).toBeVisible();
	});

	it('opens a tool-call policy on the MCP servers page', async () => {
		await renderView({ policyDirection: 'tool-calls' });

		await clickPolicy('Block shell tools');

		expect(openUrl).toHaveBeenCalledWith('/mcp-servers/ai-judge-policies/tool', false);
	});

	it('opens a user-message policy on the models page', async () => {
		await renderView({ policyDirection: 'user-message' });

		await clickPolicy('Block travel booking');

		expect(openUrl).toHaveBeenCalledWith('/models/ai-judge-policies/user', false);
	});

	it('opens a both-direction policy on the MCP servers page from the tool-call list', async () => {
		await renderView({ policyDirection: 'tool-calls' });

		await clickPolicy('Block everything');

		expect(openUrl).toHaveBeenCalledWith('/mcp-servers/ai-judge-policies/both', false);
	});

	it('opens a both-direction policy on the models page from the user-message list', async () => {
		await renderView({ policyDirection: 'user-message' });

		await clickPolicy('Block everything');

		expect(openUrl).toHaveBeenCalledWith('/models/ai-judge-policies/both', false);
	});
});

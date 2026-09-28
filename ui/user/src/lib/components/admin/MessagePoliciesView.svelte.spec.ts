import { page as appPage } from '$app/state';
import { Group, type MessagePolicy, type PolicyDirection } from '$lib/services';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import MessagePoliciesView from './MessagePoliciesView.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

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
		http.get('/api/message-policy-violations/filter-options/:filter', () => HttpResponse.json([]))
	);
}

describe('MessagePoliciesView', () => {
	it('shows only tool-call policies for MCP servers', async () => {
		await renderView({ policyDirection: 'tool-calls' });

		await expect.element(page.getByRole('row', { name: /Block shell tools/ })).toBeVisible();
		await expect
			.element(page.getByRole('row', { name: /Block travel booking/ }))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByRole('row', { name: /Block everything/ }))
			.not.toBeInTheDocument();
	});

	it('shows only user-message policies for models', async () => {
		await renderView({ policyDirection: 'user-message' });

		await expect.element(page.getByRole('row', { name: /Block travel booking/ })).toBeVisible();
		await expect
			.element(page.getByRole('row', { name: /Block shell tools/ }))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByRole('row', { name: /Block everything/ }))
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
	});
});

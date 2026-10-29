import type { ProvenanceEvent } from '$lib/services/provenance';
import { preparePageData } from '../../../tests/helpers/pageData';
import { judgedEvent, riskyEvents } from '../../../tests/mocks/provenance';
import DecisionVoice from './DecisionVoice.svelte';
import { beforeEach, describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function withDecision(decision: Partial<NonNullable<ProvenanceEvent['decision']>>) {
	return {
		...riskyEvents[1],
		decision: { allow: true, effect: 'allow', reason: '', risk: 0, ...decision }
	} as ProvenanceEvent;
}

describe('DecisionVoice', () => {
	beforeEach(async () => {
		await preparePageData();
	});

	it('speaks in the first person for a decision it judged, with its rationale', async () => {
		render(DecisionVoice, { event: judgedEvent });

		await expect
			.element(
				page.getByText(
					'I reviewed tools/call acme-crm.send_report and held it for you to decide.',
					{ exact: false }
				)
			)
			.toBeInTheDocument();
		await expect.element(page.getByText('My call')).toBeInTheDocument();
		await expect
			.element(
				page.getByText(
					'I held this because the report carries customer PII the task never asked for.'
				)
			)
			.toBeInTheDocument();
	});

	it.each([
		{
			name: 'a judged allow',
			decision: {
				reason: 'X. Supervisor (m, confidence 90%): Routine read.',
				matched: []
			},
			text: 'I reviewed tools/call github.search_issues and let it through.'
		},
		{
			name: 'a judged deny',
			decision: {
				allow: false,
				effect: 'deny' as const,
				reason: 'X. Supervisor (m, confidence 90%): Not part of the task.'
			},
			text: 'I reviewed tools/call github.search_issues and blocked it.'
		},
		{
			name: 'a policy deny',
			decision: { allow: false, effect: 'deny' as const, policyName: 'No pastes' },
			text: 'I blocked tools/call github.search_issues under No pastes.'
		},
		{
			name: 'a kill',
			decision: { allow: false, effect: 'deny' as const, killed: true },
			text: 'I stopped tools/call github.search_issues. The kill switch is on.'
		},
		{
			name: 'a honeytoken',
			decision: { allow: false, effect: 'deny' as const, honeytoken: true },
			text: 'I blocked tools/call github.search_issues. A honeytoken was in the outbound data.'
		},
		{
			name: 'a shadow match',
			decision: { effect: 'deny' as const, shadow: true, policyName: 'No pastes' },
			text: 'I let tools/call github.search_issues through. If No pastes were enforced, the outcome would be: deny.'
		},
		{
			name: 'a routine allow',
			decision: {},
			text: 'I allowed tools/call github.search_issues.'
		}
	])('tells $name', async ({ decision, text }) => {
		render(DecisionVoice, { event: withDecision(decision) });
		await expect.element(page.getByText(text, { exact: false })).toBeInTheDocument();
	});

	it('says nothing for an event without a decision', async () => {
		render(DecisionVoice, { event: riskyEvents[2] });
		await expect.element(page.getByText(/^I /)).not.toBeInTheDocument();
	});
});

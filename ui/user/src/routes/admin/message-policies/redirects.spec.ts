import { load as mcpAIJudgePolicyLoad } from '../../mcp-servers/ai-judge-policies/[id]/+page';
import { load as modelAIJudgePolicyLoad } from '../../models/ai-judge-policies/[id]/+page';
import { load as policyViolationsLoad } from '../policy-violations/+page';
import { load as messagePoliciesLoad } from './+page';
import { load as messagePolicyLoad } from './[id]/+page';
import { isRedirect } from '@sveltejs/kit';
import { describe, expect, it } from 'vitest';

function redirectOf(load: () => unknown) {
	try {
		load();
	} catch (e) {
		if (isRedirect(e)) return { status: e.status, location: e.location };
		throw e;
	}
	throw new Error('load did not redirect');
}

describe('message policy redirects', () => {
	it('sends message policies to gateway profiles, where guardrail policies are', () => {
		expect(redirectOf(messagePoliciesLoad as () => unknown)).toEqual({
			status: 307,
			location: '/models?view=profiles'
		});
		expect(redirectOf(messagePolicyLoad as () => unknown)).toEqual({
			status: 307,
			location: '/models?view=profiles'
		});
	});

	it('sends AI judge policies to gateway profiles', () => {
		for (const load of [mcpAIJudgePolicyLoad, modelAIJudgePolicyLoad]) {
			expect(redirectOf(load as () => unknown)).toEqual({
				status: 307,
				location: '/models?view=profiles'
			});
		}
	});

	it('sends policy violations to the LLM audit log', () => {
		expect(redirectOf(policyViolationsLoad as () => unknown)).toEqual({
			status: 307,
			location: '/audit-logs?view=llm'
		});
	});
});

import { requireHostedAgentsEnabled } from './hostedAgents';
import { isRedirect } from '@sveltejs/kit';
import { describe, expect, it } from 'vitest';

describe('requireHostedAgentsEnabled', () => {
	it('allows routes when Hosted Agents are enabled', () => {
		expect(() =>
			requireHostedAgentsEnabled({ hostedAgentsEnabled: true }, '/fallback')
		).not.toThrow();
	});

	it('redirects when Hosted Agents are disabled', () => {
		try {
			requireHostedAgentsEnabled({ hostedAgentsEnabled: false }, '/fallback');
			expect.unreachable('expected a redirect');
		} catch (err) {
			expect(isRedirect(err)).toBe(true);
			if (isRedirect(err)) {
				expect(err.status).toBe(302);
				expect(err.location).toBe('/fallback');
			}
		}
	});

	it('redirects when the server does not report the feature flag', () => {
		try {
			requireHostedAgentsEnabled(undefined, '/fallback');
			expect.unreachable('expected a redirect');
		} catch (err) {
			expect(isRedirect(err)).toBe(true);
		}
	});
});

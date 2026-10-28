import { matchesModelPattern } from './utils';
import { describe, expect, it } from 'vitest';

describe('matchesModelPattern', () => {
	it('matches the full target model', () => {
		expect(matchesModelPattern('anthropic/claude-haiku', 'anthropic/claude-haiku-4-5')).toBe(true);
		expect(matchesModelPattern('openai/claude-haiku', 'anthropic/claude-haiku-4-5')).toBe(false);
	});

	it('matches a prefix without a provider against the part after the provider', () => {
		expect(matchesModelPattern('claude-haiku', 'anthropic/claude-haiku-4-5')).toBe(true);
		expect(matchesModelPattern('claude-haiku', 'bedrock/claude-haiku-4-5')).toBe(true);
		expect(matchesModelPattern('claude-opus', 'anthropic/claude-haiku-4-5')).toBe(false);
	});

	it('is case-sensitive and handles a missing target model', () => {
		expect(matchesModelPattern('Claude', 'anthropic/claude-haiku-4-5')).toBe(false);
		expect(matchesModelPattern('claude', undefined)).toBe(false);
	});
});

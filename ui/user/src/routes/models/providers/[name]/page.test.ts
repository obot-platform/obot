import { load } from './+page';
import { describe, expect, it } from 'vitest';

function loadPage(name: string) {
	return load({ params: { name } } as unknown as Parameters<NonNullable<typeof load>>[0]);
}

describe('models/providers/[name] loader', () => {
	it('redirects to the provider on the Providers tab', () => {
		expect(() => loadPage('openai')).toThrow(
			expect.objectContaining({ status: 307, location: '/models?view=providers&provider=openai' })
		);
	});

	it('encodes the provider name', () => {
		expect(() => loadPage('my llm')).toThrow(
			expect.objectContaining({ location: '/models?view=providers&provider=my+llm' })
		);
	});
});

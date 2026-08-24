import Search from './Search.svelte';
import { render } from 'svelte/server';
import { describe, expect, it, vi } from 'vitest';

describe('Search SSR', () => {
	it('renders the initial value into the server HTML', () => {
		const { body } = render(Search, {
			props: { value: 'existing query', onChange: vi.fn() }
		});

		expect(body).toContain('value="existing query"');
	});
});

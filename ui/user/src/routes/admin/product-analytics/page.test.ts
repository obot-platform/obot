import { load } from './+page';
import { describe, expect, it } from 'vitest';

describe('legacy Product Analytics settings route', () => {
	it('redirects to the Product Analytics Platform tab and preserves query parameters', async () => {
		try {
			load({
				url: new URL('http://localhost/admin/product-analytics?source=bookmark')
			} as Parameters<NonNullable<typeof load>>[0]);
			expect.unreachable('Expected the legacy route to redirect');
		} catch (err) {
			expect(err).toMatchObject({
				status: 301,
				location: '/admin/platform?view=product-analytics&source=bookmark'
			});
		}
	});
});

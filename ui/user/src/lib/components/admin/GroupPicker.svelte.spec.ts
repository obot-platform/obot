import { worker } from '../../../tests/mocks/worker';
import GroupPicker from './GroupPicker.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

interface GroupPageOverrides {
	total?: number;
	degraded?: boolean;
	/** When set, any request carrying a cursor is answered with the first page and reset: true. */
	resetOnCursor?: boolean;
	/** When set, any request carrying a cursor fails, standing in for the directory going away. */
	failOnCursor?: boolean;
}

// The server hands out opaque cursors; a stringified offset is enough to stand in for one here.
function mockGroups(overrides: GroupPageOverrides = {}) {
	const total = overrides.total ?? 3;
	const requests = vi.fn();

	worker.use(
		http.get('/api/groups', ({ request }) => {
			const url = new URL(request.url);
			const name = url.searchParams.get('name') ?? '';
			const limit = Number(url.searchParams.get('limit') ?? 50);
			const cursor = url.searchParams.get('cursor');
			requests({ name, limit, cursor });

			if (overrides.failOnCursor && cursor) {
				return new HttpResponse(null, { status: 500 });
			}

			const all = Array.from({ length: total }, (_, i) => ({
				id: `entra/${String(i).padStart(4, '0')}`,
				name: `group-${String(i).padStart(4, '0')}`
			}));
			const matched = name ? all.filter((g) => g.name.includes(name)) : all;

			// The server could not honor the cursor and restarted the listing: the caller is handed
			// the first page and told its page number no longer means anything.
			const didReset = Boolean(overrides.resetOnCursor && cursor);

			const start = didReset || !cursor ? 0 : Number(cursor);
			const end = Math.min(start + limit, matched.length);

			return HttpResponse.json({
				items: matched.slice(start, end),
				nextCursor: end < matched.length ? String(end) : undefined,
				source: overrides.degraded ? 'cache' : 'provider',
				degraded: overrides.degraded ?? false,
				reset: didReset
			});
		})
	);

	return requests;
}

describe('GroupPicker', () => {
	it('renders the first page of groups', async () => {
		mockGroups({ total: 3 });
		render(GroupPicker, { onSelect: vi.fn() });

		await expect.element(page.getByText('group-0000')).toBeInTheDocument();
		await expect.element(page.getByText('group-0002')).toBeInTheDocument();
	});

	it('calls onSelect with the chosen group', async () => {
		mockGroups({ total: 3 });
		const onSelect = vi.fn();
		render(GroupPicker, { onSelect });

		await page.getByRole('button', { name: /group-0001/ }).click();

		expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: 'entra/0001' }));
	});

	it('requests a bounded page rather than the whole directory', async () => {
		const requests = mockGroups({ total: 10000 });
		render(GroupPicker, { onSelect: vi.fn(), pageSize: 50 });

		await expect.element(page.getByText('group-0000')).toBeInTheDocument();
		expect(requests).toHaveBeenCalledWith(expect.objectContaining({ limit: 50, cursor: null }));
	});

	it('pages forward through a large directory', async () => {
		const requests = mockGroups({ total: 10000 });
		render(GroupPicker, { onSelect: vi.fn(), pageSize: 50 });

		await expect.element(page.getByText('group-0000')).toBeInTheDocument();
		await page.getByRole('button', { name: /Next/ }).click();

		await expect.element(page.getByText('group-0050')).toBeInTheDocument();
		// The cursor the first page returned has to come back on the next request.
		expect(requests).toHaveBeenCalledWith(expect.objectContaining({ limit: 50, cursor: '50' }));
	});

	it('pages backward to the page it came from', async () => {
		mockGroups({ total: 10000 });
		render(GroupPicker, { onSelect: vi.fn(), pageSize: 50 });

		await expect.element(page.getByText('group-0000')).toBeInTheDocument();
		await page.getByRole('button', { name: /Next/ }).click();
		await expect.element(page.getByText('group-0050')).toBeInTheDocument();

		await page.getByRole('button', { name: /Previous/ }).click();
		await expect.element(page.getByText('group-0000')).toBeInTheDocument();
	});

	it('starts over when the search changes mid-listing', async () => {
		const requests = mockGroups({ total: 10000 });
		render(GroupPicker, { onSelect: vi.fn(), pageSize: 50 });

		await expect.element(page.getByText('group-0000')).toBeInTheDocument();
		await page.getByRole('button', { name: /Next/ }).click();
		await expect.element(page.getByText('group-0050')).toBeInTheDocument();

		// A cursor belongs to the search it was minted for, so a new search must not reuse it.
		await page.getByRole('textbox').fill('group-01');

		await vi.waitFor(() =>
			expect(requests).toHaveBeenCalledWith(
				expect.objectContaining({ name: 'group-01', cursor: null })
			)
		);
	});

	it('rewinds to the first page when the server could not honor the cursor', async () => {
		const requests = mockGroups({ total: 10000, resetOnCursor: true });
		render(GroupPicker, { onSelect: vi.fn(), pageSize: 50 });

		await expect.element(page.getByText('group-0000')).toBeInTheDocument();
		await page.getByRole('button', { name: /Next/ }).click();

		// The server restarted the listing, so this is page one again. Leaving the page number where
		// it was would label it page two and repeat every page after it.
		await expect.element(page.getByText('group-0000')).toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: /Previous/ })).toBeDisabled();

		// Rewinding must not cost an extra round trip for a page already in hand.
		const cursorRequests = requests.mock.calls.filter(([call]) => call.cursor !== null);
		expect(cursorRequests).toHaveLength(1);
	});

	it('hides Next on the last page', async () => {
		mockGroups({ total: 60 });
		render(GroupPicker, { onSelect: vi.fn(), pageSize: 50 });

		await expect.element(page.getByText('group-0000')).toBeInTheDocument();
		await page.getByRole('button', { name: /Next/ }).click();

		await expect.element(page.getByText('group-0050')).toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: /Next/ })).toBeDisabled();
	});

	it('hides pagination when everything fits on one page', async () => {
		mockGroups({ total: 3 });
		render(GroupPicker, { onSelect: vi.fn(), pageSize: 50 });

		await expect.element(page.getByText('group-0000')).toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: /Next/ })).not.toBeInTheDocument();
	});

	it('sends the search query to the server', async () => {
		const requests = mockGroups({ total: 10000 });
		render(GroupPicker, { onSelect: vi.fn() });

		await expect.element(page.getByText('group-0000')).toBeInTheDocument();
		await page.getByRole('textbox').fill('group-0123');

		await vi.waitFor(() =>
			expect(requests).toHaveBeenCalledWith(expect.objectContaining({ name: 'group-0123' }))
		);
	});

	it('excludes ids the caller asked to hide', async () => {
		mockGroups({ total: 3 });
		render(GroupPicker, { onSelect: vi.fn(), excludeIds: ['entra/0001'] });

		await expect.element(page.getByText('group-0000')).toBeInTheDocument();
		await expect.element(page.getByText('group-0001')).not.toBeInTheDocument();
	});

	it('renders the subtitle a caller provides', async () => {
		mockGroups({ total: 1 });
		render(GroupPicker, { onSelect: vi.fn(), subtitle: () => 'Admin' });

		await expect.element(page.getByText('Admin')).toBeInTheDocument();
	});

	it('warns when results fell back to cached groups', async () => {
		mockGroups({ total: 2, degraded: true });
		render(GroupPicker, { onSelect: vi.fn() });

		await expect.element(page.getByText(/groups Obot has already recorded/i)).toBeInTheDocument();
	});

	it('drops the cached-directory warning when the next page fails to load', async () => {
		mockGroups({ total: 100, degraded: true, failOnCursor: true });
		render(GroupPicker, { onSelect: vi.fn(), pageSize: 50 });

		await expect.element(page.getByText(/groups Obot has already recorded/i)).toBeInTheDocument();
		await page.getByRole('button', { name: /Next/ }).click();

		await expect.element(page.getByText(/Failed to load groups/i)).toBeInTheDocument();
		// The warning describes where the listed groups came from, and the failure listed none, so
		// showing it next to "Failed to load groups" would claim cached groups are on screen.
		await expect
			.element(page.getByText(/groups Obot has already recorded/i))
			.not.toBeInTheDocument();
	});

	it('does not warn when the provider answered', async () => {
		mockGroups({ total: 2, degraded: false });
		render(GroupPicker, { onSelect: vi.fn() });

		await expect.element(page.getByText('group-0000')).toBeInTheDocument();
		await expect
			.element(page.getByText(/groups Obot has already recorded/i))
			.not.toBeInTheDocument();
	});

	it('reports an empty search rather than looking broken', async () => {
		mockGroups({ total: 0 });
		render(GroupPicker, { onSelect: vi.fn() });

		await expect.element(page.getByText('No groups available.')).toBeInTheDocument();
	});
});

import { worker } from '../../../tests/mocks/worker';
import GroupPicker from './GroupPicker.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

interface GroupPageOverrides {
	total?: number;
	degraded?: boolean;
}

function mockGroups(overrides: GroupPageOverrides = {}) {
	const total = overrides.total ?? 3;
	const requests = vi.fn();

	worker.use(
		http.get('/api/groups', ({ request }) => {
			const url = new URL(request.url);
			const name = url.searchParams.get('name') ?? '';
			const limit = Number(url.searchParams.get('limit') ?? 50);
			const offset = Number(url.searchParams.get('offset') ?? 0);
			requests({ name, limit, offset });

			const all = Array.from({ length: total }, (_, i) => ({
				id: `entra/${String(i).padStart(4, '0')}`,
				name: `group-${String(i).padStart(4, '0')}`
			}));
			const matched = name ? all.filter((g) => g.name.includes(name)) : all;

			return HttpResponse.json({
				items: matched.slice(offset, offset + limit),
				total: matched.length,
				limit,
				offset,
				source: overrides.degraded ? 'cache' : 'provider',
				degraded: overrides.degraded ?? false
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
		expect(requests).toHaveBeenCalledWith(expect.objectContaining({ limit: 50, offset: 0 }));
	});

	it('pages forward through a large directory', async () => {
		const requests = mockGroups({ total: 10000 });
		render(GroupPicker, { onSelect: vi.fn(), pageSize: 50 });

		await expect.element(page.getByText('group-0000')).toBeInTheDocument();
		await page.getByRole('button', { name: /Next/ }).click();

		await expect.element(page.getByText('group-0050')).toBeInTheDocument();
		expect(requests).toHaveBeenCalledWith(expect.objectContaining({ limit: 50, offset: 50 }));
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

		await expect
			.element(page.getByText(/groups seen from previous sign-ins/i))
			.toBeInTheDocument();
	});

	it('does not warn when the provider answered', async () => {
		mockGroups({ total: 2, degraded: false });
		render(GroupPicker, { onSelect: vi.fn() });

		await expect.element(page.getByText('group-0000')).toBeInTheDocument();
		await expect
			.element(page.getByText(/groups seen from previous sign-ins/i))
			.not.toBeInTheDocument();
	});

	it('reports an empty search rather than looking broken', async () => {
		mockGroups({ total: 0 });
		render(GroupPicker, { onSelect: vi.fn() });

		await expect.element(page.getByText('No groups available.')).toBeInTheDocument();
	});
});

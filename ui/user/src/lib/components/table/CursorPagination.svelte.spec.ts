import CursorPagination from './CursorPagination.svelte';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

describe('CursorPagination', () => {
	it('shows a one-based page number', async () => {
		render(CursorPagination, {
			pageIndex: 2,
			hasPrevious: true,
			hasNext: true,
			onPrevious: vi.fn(),
			onNext: vi.fn()
		});

		await expect.element(page.getByText('Page 3')).toBeInTheDocument();
	});

	it('disables Previous on the first page', async () => {
		render(CursorPagination, {
			pageIndex: 0,
			hasPrevious: false,
			hasNext: true,
			onPrevious: vi.fn(),
			onNext: vi.fn()
		});

		await expect.element(page.getByRole('button', { name: /Previous/ })).toBeDisabled();
		await expect.element(page.getByRole('button', { name: /Next/ })).not.toBeDisabled();
	});

	// Without a total, the absence of a next cursor is the only way to know this is the last page.
	it('disables Next on the last page', async () => {
		render(CursorPagination, {
			pageIndex: 3,
			hasPrevious: true,
			hasNext: false,
			onPrevious: vi.fn(),
			onNext: vi.fn()
		});

		await expect.element(page.getByRole('button', { name: /Next/ })).toBeDisabled();
		await expect.element(page.getByRole('button', { name: /Previous/ })).not.toBeDisabled();
	});

	it('disables both controls while a page is loading', async () => {
		render(CursorPagination, {
			pageIndex: 1,
			hasPrevious: true,
			hasNext: true,
			loading: true,
			onPrevious: vi.fn(),
			onNext: vi.fn()
		});

		await expect.element(page.getByRole('button', { name: /Previous/ })).toBeDisabled();
		await expect.element(page.getByRole('button', { name: /Next/ })).toBeDisabled();
	});

	// A disabled control has to look disabled, not just refuse the click.
	it('dims the control it disables', async () => {
		render(CursorPagination, {
			pageIndex: 0,
			hasPrevious: false,
			hasNext: true,
			onPrevious: vi.fn(),
			onNext: vi.fn()
		});

		const previous = page.getByRole('button', { name: /Previous/ });
		await expect.element(previous).toBeDisabled();
		await expect.element(previous).toHaveStyle({ opacity: '0.5' });

		const next = page.getByRole('button', { name: /Next/ });
		await expect.element(next).toHaveStyle({ opacity: '1' });
	});

	it('reports which direction was clicked', async () => {
		const onPrevious = vi.fn();
		const onNext = vi.fn();
		render(CursorPagination, {
			pageIndex: 1,
			hasPrevious: true,
			hasNext: true,
			onPrevious,
			onNext
		});

		await page.getByRole('button', { name: /Next/ }).click();
		expect(onNext).toHaveBeenCalledOnce();

		await page.getByRole('button', { name: /Previous/ }).click();
		expect(onPrevious).toHaveBeenCalledOnce();
	});
});

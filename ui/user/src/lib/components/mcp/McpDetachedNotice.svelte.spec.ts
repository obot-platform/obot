import McpDetachedNotice from './McpDetachedNotice.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

describe('Detached Git source links', () => {
	it.each([
		{
			sourceURL: 'https://bitbucket.org/workspace/catalog.git/staging',
			href: 'https://bitbucket.org/workspace/catalog/src/staging/'
		},
		{
			sourceURL: 'https://bitbucket.org/workspace/catalog.git',
			href: 'https://bitbucket.org/workspace/catalog/'
		},
		{
			sourceURL: 'https://github.com/org/catalog',
			href: 'https://github.com/org/catalog'
		}
	])('links $sourceURL to $href', async ({ sourceURL, href }) => {
		render(McpDetachedNotice, { detached: true, variant: 'notification', sourceURL });
		await expect
			.element(page.getByRole('link', { name: 'View original Git source' }))
			.toHaveAttribute('href', href);
	});

	it('displays local sources without a link', async () => {
		render(McpDetachedNotice, {
			detached: true,
			variant: 'notification',
			sourceURL: '/catalog/local'
		});
		await expect.element(page.getByText('Original source: /catalog/local')).toBeVisible();
		await expect
			.element(page.getByRole('link', { name: 'View original Git source' }))
			.not.toBeInTheDocument();
	});
});

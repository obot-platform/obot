import SourceUrlsView from './SourceUrlsView.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

describe('Git source browser links', () => {
	it.each([
		{
			source: 'https://bitbucket.org/workspace/catalog.git/staging',
			href: 'https://bitbucket.org/workspace/catalog/src/staging/'
		},
		{
			source: 'https://bitbucket.org/workspace/catalog.git',
			href: 'https://bitbucket.org/workspace/catalog/'
		},
		{
			source: 'https://bitbucket.org/workspace/catalog.git/feature/catalog',
			href: 'https://bitbucket.org/workspace/catalog/src/feature/catalog/'
		},
		{
			source: 'https://github.com/org/catalog.git/main',
			href: 'https://github.com/org/catalog.git/main'
		},
		{
			source: 'https://bitbucket.org/workspace/catalog/src/main/',
			href: 'https://bitbucket.org/workspace/catalog/src/main/'
		}
	])('links $source to $href while preserving the source label', async ({ source, href }) => {
		render(SourceUrlsView, {
			catalog: {
				id: 'default',
				displayName: 'Default',
				sourceURLs: [source],
				allowedUserIDs: []
			},
			readonly: true
		});
		await expect
			.element(page.getByRole('link', { name: source, exact: true }))
			.toHaveAttribute('href', href);
	});
});

import type { CompositeServerToolRow } from '$lib/services';
import { renderOpenDialog } from '../../../../tests/helpers/openDialog';
import { reactive } from '../../../../tests/helpers/reactive.svelte';
import CompositeEditTools from './CompositeEditTools.svelte';
import { describe, expect, it } from 'vitest';
import { page } from 'vitest/browser';

function toolRows(): CompositeServerToolRow[] {
	return reactive([
		{
			id: 'list-issues',
			name: 'list_issues',
			description: 'List repository issues',
			enabled: true
		},
		{
			id: 'create-issue',
			name: 'create_issue',
			description: 'Create a repository issue',
			enabled: false
		},
		{
			id: 'list-pulls',
			name: 'list_pulls',
			description: 'List pull requests',
			enabled: false
		},
		{
			id: 'merge-pull',
			name: 'merge_pull',
			description: 'Merge a pull request',
			enabled: true
		},
		{
			id: 'old-issue-tool',
			name: 'old_issue_tool',
			description: 'Removed issue tool',
			enabled: false,
			removed: true
		}
	]);
}

function enableAllToggle() {
	return page.getByRole('switch', { name: 'Enable All Tools' });
}

function toolToggle(id: string) {
	return page.getByCSS(`#edit-tool-${id} [role="switch"]`);
}

function enabledById(tools: CompositeServerToolRow[]) {
	return Object.fromEntries(tools.map((tool) => [tool.id, tool.enabled]));
}

async function searchTools(query: string) {
	await page.getByPlaceholder('Search tools...').fill(query);
}

async function renderTools(tools: CompositeServerToolRow[]) {
	return renderOpenDialog(CompositeEditTools, { name: 'GitHub', tools });
}

describe('CompositeEditTools Enable All Tools', () => {
	describe('without a search query', () => {
		it('enables every actionable tool and leaves removed tools alone', async () => {
			const tools = toolRows();
			await renderTools(tools);

			await expect.element(enableAllToggle()).not.toBeChecked();

			await enableAllToggle().click();

			await expect.element(enableAllToggle()).toBeChecked();
			expect(enabledById(tools)).toEqual({
				'list-issues': true,
				'create-issue': true,
				'list-pulls': true,
				'merge-pull': true,
				'old-issue-tool': false
			});
			for (const id of ['list-issues', 'create-issue', 'list-pulls', 'merge-pull']) {
				await expect.element(toolToggle(id)).toBeChecked();
			}
		});

		it('disables every actionable tool when toggled off', async () => {
			const tools = toolRows();
			for (const tool of tools) if (!tool.removed) tool.enabled = true;
			await renderTools(tools);

			await expect.element(enableAllToggle()).toBeChecked();

			await enableAllToggle().click();

			await expect.element(enableAllToggle()).not.toBeChecked();
			expect(enabledById(tools)).toEqual({
				'list-issues': false,
				'create-issue': false,
				'list-pulls': false,
				'merge-pull': false,
				'old-issue-tool': false
			});
		});
	});

	describe('with a search query', () => {
		it('reflects only the tools that match the query', async () => {
			const tools = toolRows();
			// Every tool matching "pull" is off, while some other tool is on.
			for (const tool of tools) if (tool.id === 'merge-pull') tool.enabled = false;
			await renderTools(tools);

			await expect.element(enableAllToggle()).not.toBeChecked();

			// Every actionable tool matching "issue" is on; old_issue_tool is removed and ignored.
			tools.find((tool) => tool.id === 'create-issue')!.enabled = true;
			await searchTools('issue');

			await expect.element(page.getByCSS('#edit-tool-list-pulls')).not.toBeInTheDocument();
			await expect.element(enableAllToggle()).toBeChecked();
		});

		it('enables only the tools that match the query', async () => {
			const tools = toolRows();
			await renderTools(tools);

			await searchTools('pull');
			await expect.element(page.getByCSS('#edit-tool-list-issues')).not.toBeInTheDocument();
			await expect.element(enableAllToggle()).not.toBeChecked();

			await enableAllToggle().click();

			await expect.element(enableAllToggle()).toBeChecked();
			expect(enabledById(tools)).toEqual({
				'list-issues': true,
				'create-issue': false,
				'list-pulls': true,
				'merge-pull': true,
				'old-issue-tool': false
			});

			// Clearing the query shows the full list, which is not all enabled.
			await searchTools('');
			await expect.element(page.getByCSS('#edit-tool-create-issue')).toBeVisible();
			await expect.element(enableAllToggle()).not.toBeChecked();
		});

		it('disables only the tools that match the query', async () => {
			const tools = toolRows();
			for (const tool of tools) if (!tool.removed) tool.enabled = true;
			await renderTools(tools);

			await searchTools('issue');
			await expect.element(page.getByCSS('#edit-tool-list-pulls')).not.toBeInTheDocument();
			await expect.element(enableAllToggle()).toBeChecked();

			await enableAllToggle().click();

			await expect.element(enableAllToggle()).not.toBeChecked();
			expect(enabledById(tools)).toEqual({
				'list-issues': false,
				'create-issue': false,
				'list-pulls': true,
				'merge-pull': true,
				'old-issue-tool': false
			});

			await searchTools('');
			await expect.element(page.getByCSS('#edit-tool-list-pulls')).toBeVisible();
			await expect.element(toolToggle('list-pulls')).toBeChecked();
			await expect.element(toolToggle('merge-pull')).toBeChecked();
			await expect.element(enableAllToggle()).not.toBeChecked();
		});

		it('is disabled when the query matches only removed tools', async () => {
			const tools = toolRows();
			await renderTools(tools);

			await searchTools('old_issue');

			await expect.element(page.getByCSS('#edit-tool-old-issue-tool')).toBeVisible();
			await expect.element(enableAllToggle()).toBeDisabled();
		});

		it('is off when the query matches no actionable tools', async () => {
			const tools = toolRows();
			for (const tool of tools) if (!tool.removed) tool.enabled = true;
			await renderTools(tools);

			await expect.element(enableAllToggle()).toBeChecked();

			await searchTools('old_issue');
			await expect.element(enableAllToggle()).toBeDisabled();
			await expect.element(enableAllToggle()).not.toBeChecked();

			await searchTools('zzz');
			await expect.element(page.getByCSS('#edit-tool-old-issue-tool')).not.toBeInTheDocument();
			await expect.element(enableAllToggle()).toBeDisabled();
			await expect.element(enableAllToggle()).not.toBeChecked();
		});
	});
});

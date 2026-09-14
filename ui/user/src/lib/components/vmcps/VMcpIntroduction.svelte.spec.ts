import VMcpIntroduction from './VMcpIntroduction.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const STORAGE_KEY = '@obot/seen-vmcp-introduction';

function heading() {
	return page.getByRole('heading', { name: 'Welcome to vMCP Designer', exact: true });
}

function getStartedButton() {
	return page.getByRole('button', { name: 'Get started', exact: true });
}

describe('VMcpIntroduction.svelte', () => {
	it('shows the introduction on a first visit', async () => {
		render(VMcpIntroduction);

		await expect.element(page.getByRole('dialog')).toBeVisible();
		await expect.element(heading()).toBeVisible();
		await expect
			.element(
				page.getByText(
					'What is a vMCP? A virtual MCP (vMCP) exposes one or more MCP servers through one Obot Gateway endpoint.'
				)
			)
			.toBeVisible();
		await expect.element(page.getByText('Create New vMCP')).toBeVisible();
		await expect.element(page.getByText('MCP Server', { exact: true })).toBeVisible();
	});

	it('stays hidden once it has been seen', async () => {
		localStorage.setItem(STORAGE_KEY, new Date().toISOString());
		render(VMcpIntroduction);

		await expect.element(heading()).not.toBeInTheDocument();
	});

	it('remembers a dismissal so it does not come back', async () => {
		render(VMcpIntroduction);

		await getStartedButton().click();

		await expect.element(heading()).not.toBeInTheDocument();
		expect(localStorage.getItem(STORAGE_KEY)).not.toBeNull();
	});
});

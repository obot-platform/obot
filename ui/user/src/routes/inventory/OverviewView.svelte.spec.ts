import { page as appPage } from '$app/state';
import OverviewView from './OverviewView.svelte';
import { afterEach, describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const range = {
	start: '2026-09-18T00:00:00Z',
	end: '2026-09-25T00:00:00Z'
};

const stats = {
	timeStart: range.start,
	timeEnd: range.end,
	deviceCount: 2,
	userCount: 1,
	clients: [
		{ name: 'workbuddy', deviceCount: 2, userCount: 1, observationCount: 2 },
		{ name: 'opencode', deviceCount: 1, userCount: 1, observationCount: 1 },
		{ name: 'zcode', deviceCount: 1, userCount: 1, observationCount: 1 }
	],
	mcpServers: [],
	skills: [],
	scanTimestamps: [range.start, range.end]
};

afterEach(() => {
	appPage.url.searchParams.delete('start');
	appPage.url.searchParams.delete('end');
});

describe('OverviewView client labels', () => {
	it('shows branded client names in device scan statistics', async () => {
		render(OverviewView, { stats, range });

		await expect.element(page.getByText('WorkBuddy', { exact: true })).toBeVisible();
		await expect.element(page.getByText('OpenCode', { exact: true })).toBeVisible();
		await expect.element(page.getByText('ZCode', { exact: true })).toBeVisible();
		await expect.element(page.getByText('workbuddy', { exact: true })).not.toBeInTheDocument();
		await expect.element(page.getByText('opencode', { exact: true })).not.toBeInTheDocument();
		await expect.element(page.getByText('zcode', { exact: true })).not.toBeInTheDocument();
	});
});

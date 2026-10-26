import { goto } from '$app/navigation';
import { page as appPage } from '$app/state';
import { Group, UserService, type ClaudeSession, type OrgUser } from '$lib/services';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import { getProfileResponse } from '../../../tests/mocks/data';
import ClaudeSessions from './ClaudeSessions.svelte';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { cleanup, render } from 'vitest-browser-svelte';
import { page, userEvent } from 'vitest/browser';

vi.mock('$app/state', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$app/state')>();
	const { createAppState } = await import('../../../tests/helpers/navigation.svelte');
	return createAppState(actual);
});

vi.mock('$app/navigation', async () => {
	const { applyTestGoto } = await import('../../../tests/helpers/navigation.svelte');
	return {
		goto: vi.fn((url: string | URL) => applyTestGoto(url)),
		invalidate: vi.fn(),
		invalidateAll: vi.fn(),
		preloadCode: vi.fn(),
		preloadData: vi.fn(),
		beforeNavigate: vi.fn(),
		afterNavigate: vi.fn(),
		onNavigate: vi.fn(),
		pushState: vi.fn(),
		replaceState: vi.fn(),
		disableScrollHandling: vi.fn()
	};
});

const sessionUrlKeys = [
	'sort',
	'sortDirection',
	'query',
	'status',
	'user',
	'variant',
	'showMySessionsOnly'
];

const repository = 'https://github.com/obot-platform/obot (main)';
const slackSummary = 'Donnie Adams @thedadams · #craig-test';

beforeEach(() => {
	for (const key of sessionUrlKeys) (appPage.url as URL).searchParams.delete(key);
});

afterEach(() => {
	cleanup();
	vi.restoreAllMocks();
	for (const key of sessionUrlKeys) (appPage.url as URL).searchParams.delete(key);
});

function mailboxSession(): ClaudeSession {
	return {
		metadata: { name: 'session-one' },
		spec: {
			claudeAccount: 'acct',
			claudeEnvironment: 'env-one',
			claudeSession: 'claude-one',
			displayName: 'Mailbox agent',
			userID: getProfileResponse.id,
			claudeDone: true,
			claudeFinishedAt: '2026-01-01T12:00:00Z',
			claudeRepositories: [{ url: 'https://github.com/obot-platform/obot', revision: 'main' }],
			slackSenderName: 'Donnie Adams',
			slackSenderHandle: 'thedadams',
			slackChannelName: 'craig-test',
			slackLink: 'https://acorn-io.slack.com/archives/C123/p123?thread_ts=1.23'
		},
		status: { state: 'RUNNING', error: 'Runner unavailable' }
	};
}

async function renderSessions(all = true) {
	await preparePageData({
		profile: createMockProfile([Group.ADMIN])
	});
	vi.spyOn(UserService, 'listClaudeSessions').mockResolvedValue([mailboxSession()]);
	vi.spyOn(UserService, 'listUsersIncludeDeleted').mockResolvedValue([
		{
			id: getProfileResponse.id,
			displayName: 'Ada Admin',
			email: 'ada@example.com',
			username: 'ada',
			created: '',
			explicitRole: true,
			role: 1,
			effectiveRole: 1,
			groups: [Group.ADMIN],
			iconURL: ''
		} satisfies OrgUser
	]);
	return render(ClaudeSessions, { all });
}

it('keeps repository and slack details on the session card', async () => {
	await renderSessions();

	await expect
		.element(page.getByRole('heading', { level: 2, name: 'Mailbox agent' }))
		.toBeVisible();
	await expect.element(page.getByText('main', { exact: true })).toBeVisible();
	await expect.element(page.getByText('@thedadams', { exact: true })).toBeVisible();
	await expect
		.element(page.getByRole('link', { name: 'Open Slack thread' }))
		.toHaveAttribute('href', 'https://acorn-io.slack.com/archives/C123/p123?thread_ts=1.23');
});

it('updates the search url and keeps focus when searching and clearing', async () => {
	(appPage.url as URL).searchParams.set('status', 'suspended');
	await renderComparableSessions();
	const search = page.getByRole('textbox');
	vi.mocked(goto).mockClear();

	await search.fill('Alpha, agent');
	await vi.waitFor(() => {
		expect((appPage.url as URL).searchParams.get('query')).toBe('Alpha, agent');
		expect(goto).toHaveBeenCalledWith(
			expect.anything(),
			expect.objectContaining({ replace: true })
		);
	});
	const [url] = vi.mocked(goto).mock.lastCall!;
	expect(String(url)).toContain('query=Alpha%2C+agent');
	expect((appPage.url as URL).searchParams.get('status')).toBe('suspended');
	await expect.element(search).toHaveFocus();
	await expect.element(page.getByText('Alpha agent', { exact: true })).not.toBeInTheDocument();

	vi.mocked(goto).mockClear();
	await search.fill('');
	await vi.waitFor(() => {
		expect((appPage.url as URL).searchParams.has('query')).toBe(false);
		expect(goto).toHaveBeenCalledWith(
			expect.anything(),
			expect.objectContaining({ replace: true })
		);
	});
	const [clearedUrl] = vi.mocked(goto).mock.lastCall!;
	expect(String(clearedUrl)).not.toContain('query=');
	expect((appPage.url as URL).searchParams.get('status')).toBe('suspended');
	await expect.element(search).toHaveFocus();
	await expect.element(page.getByText('Alpha agent', { exact: true })).toBeVisible();
});

it('shows truncated repository and slack text with links and delete in the row menu', async () => {
	await renderSessions();
	await page.getByRole('button', { name: 'Table View' }).click();

	await expect.element(page.getByRole('cell', { name: 'Repositories', exact: true })).toBeVisible();
	await expect.element(page.getByRole('cell', { name: 'Slack', exact: true })).toBeVisible();
	await expect.element(page.getByRole('cell', { name: 'Obot user', exact: true })).toBeVisible();
	await expect.element(page.getByText(repository, { exact: true })).toBeVisible();
	await expect.element(page.getByText(slackSummary, { exact: true })).toBeVisible();
	await expect.element(page.getByRole('button', { name: 'Runner unavailable' })).toBeVisible();
	await expect.element(page.getByText('main', { exact: true })).not.toBeInTheDocument();

	await page.getByRole('button', { name: 'Actions for Mailbox agent' }).click();
	await expect
		.element(page.getByRole('link', { name: 'Open in Claude' }))
		.toHaveAttribute('href', 'https://claude.ai/code/claude-one');
	await expect
		.element(page.getByRole('link', { name: 'Open Slack thread' }))
		.toHaveAttribute('href', 'https://acorn-io.slack.com/archives/C123/p123?thread_ts=1.23');
	await expect.element(page.getByRole('button', { name: 'Suspend', exact: true })).toBeVisible();

	await page.getByRole('button', { name: 'Delete session', exact: true }).click();
	await expect
		.element(
			page.getByText(
				'All execution attempts and saved workspace files will be permanently removed.'
			)
		)
		.toBeVisible();
});

function orgUser(id: string, displayName: string, email: string): OrgUser {
	return {
		id,
		displayName,
		email,
		username: email.split('@')[0] ?? displayName,
		created: '',
		explicitRole: true,
		role: 1,
		effectiveRole: 1,
		groups: [Group.ADMIN],
		iconURL: ''
	};
}

function namedSession(options: {
	name: string;
	displayName: string;
	userID: string;
	state: string;
	finishedAt: string;
}): ClaudeSession {
	const base = mailboxSession();
	return {
		...base,
		metadata: { name: options.name },
		spec: {
			...base.spec,
			displayName: options.displayName,
			userID: options.userID,
			claudeSession: options.name,
			claudeDone: true,
			claudeFinishedAt: options.finishedAt
		},
		status: { state: options.state }
	};
}

async function renderComparableSessions() {
	await preparePageData({
		profile: createMockProfile([Group.ADMIN])
	});
	const ada = orgUser(getProfileResponse.id, 'Ada Admin', 'ada@example.com');
	const bea = orgUser('user-bea', 'Bea Zulu', 'bea@example.com');
	vi.spyOn(UserService, 'listClaudeSessions').mockResolvedValue([
		namedSession({
			name: 'alpha',
			displayName: 'Alpha agent',
			userID: ada.id,
			state: 'SUSPENDED',
			finishedAt: '2026-01-01T12:00:00Z'
		}),
		namedSession({
			name: 'zulu',
			displayName: 'Zulu agent',
			userID: bea.id,
			state: 'RUNNING',
			finishedAt: '2026-06-01T12:00:00Z'
		})
	]);
	vi.spyOn(UserService, 'listUsersIncludeDeleted').mockResolvedValue([ada, bea]);
	return render(ClaudeSessions, { all: true });
}

async function showTable() {
	await page.getByRole('button', { name: 'Table View' }).click();
	await expect.element(page.getByRole('cell', { name: 'Name', exact: true })).toBeVisible();
}

async function dismissOverlays() {
	for (let attempt = 0; attempt < 3; attempt++) {
		const overlay = page.getByCSS('#click-catch');
		let element: Element;
		try {
			element = await overlay.element();
		} catch {
			return;
		}
		if (!(element instanceof HTMLElement)) return;
		element.click();
	}
}

async function rowTop(name: string) {
	const element = await page.getByText(name, { exact: true }).element();
	return element.getBoundingClientRect().top;
}

it('applies status filters from the url to the table and settings', async () => {
	(appPage.url as URL).searchParams.set('variant', 'table');
	(appPage.url as URL).searchParams.set('status', 'suspended');
	await renderComparableSessions();

	await expect.element(page.getByText('Alpha agent', { exact: true })).toBeVisible();
	await expect.element(page.getByText('Zulu agent', { exact: true })).not.toBeInTheDocument();
	await expect.element(page.getByRole('button', { name: 'Remove Suspended' })).toBeVisible();
});

it('keeps table status filters and settings pills on the same url value', async () => {
	await renderComparableSessions();
	await showTable();

	await page.getByRole('button', { name: 'Status', exact: true }).click();
	await page.getByRole('combobox', { name: 'Filter by Status...' }).click();
	await page.getByRole('button', { name: 'Suspended', exact: true }).click();

	expect((appPage.url as URL).searchParams.get('status')).toBe('suspended');
	await expect.element(page.getByText('Zulu agent', { exact: true })).not.toBeInTheDocument();
	await expect.element(page.getByRole('button', { name: 'Remove Suspended' })).toBeVisible();
	await dismissOverlays();

	await page.getByRole('button', { name: 'Grid View' }).click();
	await expect.element(page.getByRole('heading', { level: 2, name: 'Alpha agent' })).toBeVisible();
	await expect
		.element(page.getByRole('heading', { level: 2, name: 'Zulu agent' }))
		.not.toBeInTheDocument();
});

it('filters by user from settings with the same id the table uses', async () => {
	await renderComparableSessions();
	await showTable();
	await expect
		.element(page.getByRole('cell', { name: 'Bea Zulu (bea@example.com)' }))
		.toBeVisible();

	await page.getByRole('button', { name: 'Filters' }).click();
	await page.getByRole('combobox', { name: 'Filter By User' }).click();
	await page.getByRole('button', { name: 'Bea Zulu (bea@example.com)', exact: true }).click();

	expect((appPage.url as URL).searchParams.get('user')).toBe('user-bea');
	await userEvent.keyboard('{Escape}');
	await expect.element(page.getByText('Alpha agent', { exact: true })).not.toBeInTheDocument();
	await expect.element(page.getByText('Zulu agent', { exact: true })).toBeVisible();
	await expect
		.element(page.getByRole('button', { name: 'Remove Bea Zulu (bea@example.com)' }))
		.toBeVisible();
});

it('writes table name sorting to the url and the settings sort controls', async () => {
	await renderComparableSessions();
	await showTable();
	expect(await rowTop('Zulu agent')).toBeLessThan(await rowTop('Alpha agent'));

	await page.getByRole('columnheader', { name: 'Name', exact: true }).click();

	expect((appPage.url as URL).searchParams.get('sort')).toBe('displayName');
	expect((appPage.url as URL).searchParams.get('sortDirection')).toBe('asc');
	expect(await rowTop('Alpha agent')).toBeLessThan(await rowTop('Zulu agent'));

	await page.getByRole('button', { name: 'Filters' }).click();
	await expect.element(page.getByRole('combobox', { name: 'Sort By' })).toHaveTextContent('Name');
	await expect
		.element(page.getByRole('combobox', { name: 'Sort Direction' }))
		.toHaveTextContent('Ascending');
});

it('hides owner columns when the list is not showing every session', async () => {
	await renderSessions(false);
	await page.getByRole('button', { name: 'Table View' }).click();

	await expect.element(page.getByRole('cell', { name: 'Name', exact: true })).toBeVisible();
	await expect
		.element(page.getByRole('cell', { name: 'Obot user', exact: true }))
		.not.toBeInTheDocument();
	await expect
		.element(page.getByRole('cell', { name: 'Environment', exact: true }))
		.not.toBeInTheDocument();
	await expect.element(page.getByText(repository, { exact: true })).toBeVisible();
});

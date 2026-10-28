import { page as appPage } from '$app/state';
import { setLocale } from '$lib/paraglide/runtime';
import { AdminService, Group, UserService } from '$lib/services';
import { profile } from '$lib/stores';
import { createMockProfile } from '../../../../tests/helpers/pageData';
import EgressAuditLogsContent from '../../../../tests/mocks/EgressAuditLogsContent.harness.svelte';
import { worker } from '../../../../tests/mocks/worker';
import { http, HttpResponse } from 'msw';
import { afterEach, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page, userEvent } from 'vitest/browser';

vi.mock('$app/state', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$app/state')>();
	const { createAppState } = await import('../../../../tests/helpers/navigation.svelte');
	return createAppState(actual);
});

vi.mock('$app/navigation', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$app/navigation')>();
	const { applyTestGoto } = await import('../../../../tests/helpers/navigation.svelte');
	return {
		...actual,
		goto: (url: string | URL) => applyTestGoto(url),
		replaceState: (url: string | URL) => applyTestGoto(url)
	};
});

vi.mock('$lib/url', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/url')>();
	const { applyTestGoto } = await import('../../../../tests/helpers/navigation.svelte');
	return {
		...actual,
		goto: (url: string | URL) => applyTestGoto(url),
		replaceState: (url: string | URL) => applyTestGoto(url)
	};
});

afterEach(() => {
	setLocale('en', { reload: false });
	for (const key of [
		'query',
		'user_id',
		'environment_id',
		'claude_account',
		'claude_session',
		'host',
		'outcome',
		'start_time',
		'end_time'
	]) {
		appPage.url.searchParams.delete(key);
	}
});

const west = {
	id: 'egress-west',
	createdAt: new Date().toISOString(),
	userID: '7',
	claudeAccount: 'user_west',
	environmentID: 'env-west',
	claudeSession: 'session_west',
	method: 'GET',
	host: 'example.com',
	path: '/west',
	responseStatus: 200,
	outcome: 'complete',
	requestBytes: 0,
	responseBytes: 8,
	durationMs: 42
};
const east = {
	...west,
	id: 'egress-east',
	claudeAccount: 'user_east',
	environmentID: 'env-east',
	claudeSession: 'session_east',
	path: '/east'
};

function mockEgressApis() {
	worker.use(
		http.get('*/api/egress-audit-logs/environments', () =>
			HttpResponse.json({
				'env-west': 'Production US West',
				'env-east': 'Production US East'
			})
		),
		http.get('*/api/egress-audit-logs', ({ request }) => {
			const environmentID = new URL(request.url).searchParams.get('environment_id');
			const items = [west, east].filter(
				(entry) => !environmentID || entry.environmentID === environmentID
			);
			return HttpResponse.json({ items, total: items.length });
		}),
		http.get('*/api/egress-audit-logs/egress-west', () =>
			HttpResponse.json({ ...west, responseBody: btoa('private response') })
		)
	);
}

for (const translation of [
	{
		locale: 'ja',
		empty: '環境の監査ログはありません',
		request: 'リクエスト',
		response: 'レスポンス',
		bytes: '8 バイト'
	},
	{
		locale: 'ko',
		empty: '환경 감사 로그가 없습니다',
		request: '요청',
		response: '응답',
		bytes: '8바이트'
	},
	{ locale: 'zh-CN', empty: '暂无环境审计日志', request: '请求', response: '响应', bytes: '8 字节' }
] as const) {
	it(`localizes the environment audit empty state in ${translation.locale}`, async () => {
		setLocale(translation.locale, { reload: false });
		profile.initialize(createMockProfile([Group.ADMIN]));
		vi.spyOn(UserService, 'listUsersIncludeDeleted').mockResolvedValue([]);
		mockEgressApis();
		worker.use(
			http.get('*/api/egress-audit-logs', () => HttpResponse.json({ items: [], total: 0 }))
		);
		render(EgressAuditLogsContent);
		await expect
			.element(page.getByRole('heading', { name: translation.empty, exact: true }))
			.toBeVisible();
	});

	it(`localizes request, response, and byte counts in ${translation.locale}`, async () => {
		setLocale(translation.locale, { reload: false });
		profile.initialize(createMockProfile([Group.ADMIN]));
		vi.spyOn(UserService, 'listUsersIncludeDeleted').mockResolvedValue([]);
		mockEgressApis();
		render(EgressAuditLogsContent);
		await page.getByRole('row').filter({ hasText: 'user_west' }).first().click();
		const response = page
			.getByCSS('#egress-audit-logs-details-sidebar p')
			.filter({ hasText: translation.bytes });
		await expect
			.element(response)
			.toHaveTextContent(`${translation.response} ${translation.bytes}`);
		await expect
			.element(
				page
					.getByCSS('#egress-audit-logs-details-sidebar p')
					.filter({ hasText: translation.request })
					.filter({ hasText: '0' })
			)
			.toBeVisible();
	});
}

for (const key of ['{Enter}', ' ']) {
	it(`opens egress details from the focused row using ${key === ' ' ? 'Space' : 'Enter'}`, async () => {
		profile.initialize(createMockProfile([Group.ADMIN]));
		vi.spyOn(UserService, 'listUsersIncludeDeleted').mockResolvedValue([]);
		const inspect = vi.spyOn(AdminService, 'getEgressAuditLog');
		mockEgressApis();
		render(EgressAuditLogsContent);

		const row = page.getByRole('row').filter({ hasText: 'user_west' }).first();
		const nextRow = page.getByRole('row').filter({ hasText: 'user_east' }).first();
		await expect.element(row).toBeVisible();
		await expect.element(nextRow).toBeVisible();
		await expect.element(row).toHaveAttribute('tabindex', '0');
		await expect.element(row.getByRole('button')).not.toBeInTheDocument();
		row.element().focus();
		await expect.element(row).toHaveFocus();
		await userEvent.tab();
		await expect.element(nextRow).toHaveFocus();
		await userEvent.tab({ shift: true });
		await expect.element(row).toHaveFocus();
		await userEvent.keyboard(key);
		expect(inspect).toHaveBeenCalledExactlyOnceWith('egress-west');
		await expect
			.element(
				page.getByCSS('#egress-audit-logs-details-sidebar p').filter({ hasText: 'Claude session' })
			)
			.toHaveTextContent('Claude session session_west');
	});
}

for (const auditor of [false, true]) {
	it(`opens egress details from a row and ${auditor ? 'reveals' : 'hides'} payloads for ${auditor ? 'auditors' : 'admins'}`, async () => {
		profile.initialize(createMockProfile(auditor ? [Group.ADMIN, Group.AUDITOR] : [Group.ADMIN]));
		vi.spyOn(UserService, 'listUsersIncludeDeleted').mockResolvedValue([]);
		mockEgressApis();
		render(EgressAuditLogsContent);

		const row = page.getByRole('row').filter({ hasText: 'user_west' }).first();
		await expect.element(row).toBeVisible();
		await expect.element(row.getByText('session_west', { exact: true })).toBeVisible();

		await row.click();
		await expect
			.poll(() =>
				document.querySelector('#egress-audit-logs-details-sidebar')?.matches(':popover-open')
			)
			.toBe(true);
		await expect
			.element(
				page.getByCSS('#egress-audit-logs-details-sidebar p').filter({ hasText: 'Claude session' })
			)
			.toHaveTextContent('Claude session session_west');
		if (auditor) {
			await expect.element(page.getByText('private response', { exact: false })).toBeVisible();
		} else {
			await expect
				.element(page.getByText('private response', { exact: false }))
				.not.toBeInTheDocument();
		}
	});
}

it('keeps drawer filter edits local until apply, then shows a removable pill', async () => {
	profile.initialize(createMockProfile([Group.ADMIN]));
	vi.spyOn(UserService, 'listUsersIncludeDeleted').mockResolvedValue([]);
	mockEgressApis();
	render(EgressAuditLogsContent);

	await expect.element(page.getByText('Production US West', { exact: true }).first()).toBeVisible();
	await expect.element(page.getByText('user_east', { exact: true }).first()).toBeVisible();
	await page.getByRole('button', { name: 'Filters', exact: true }).click();
	await page.getByCSS('#filter-environment_id').click();
	await page.getByRole('button', { name: 'Production US West', exact: true }).click();
	await expect.element(page.getByText('user_east', { exact: true }).first()).toBeVisible();

	await page.getByRole('button', { name: 'Apply Filters', exact: true }).click();
	await expect
		.element(page.getByText('user_east', { exact: true }).first())
		.not.toBeInTheDocument();
	await expect.element(page.getByText('user_west', { exact: true }).first()).toBeVisible();

	await page.getByRole('button', { name: 'Remove Environment filter', exact: true }).click();
	await expect.element(page.getByText('user_east', { exact: true }).first()).toBeVisible();
});

it('applies, restores, and clears free-text account, session, and host filters', async () => {
	profile.initialize(createMockProfile([Group.ADMIN]));
	vi.spyOn(UserService, 'listUsersIncludeDeleted').mockResolvedValue([]);
	mockEgressApis();
	const requests = vi.fn();
	const values = {
		claude_account: 'account,unknown',
		claude_session: 'session/arbitrary',
		host: 'api.example.com:8443'
	};
	const empty = { claude_account: null, claude_session: null, host: null };
	worker.use(
		http.get('*/api/egress-audit-logs', ({ request }) => {
			const params = new URL(request.url).searchParams;
			requests(Object.fromEntries(Object.keys(values).map((key) => [key, params.get(key)])));
			return HttpResponse.json({ items: [west], total: 1 });
		})
	);
	render(EgressAuditLogsContent);
	await vi.waitFor(() => expect(requests).toHaveBeenLastCalledWith(empty));
	await page.getByRole('button', { name: 'Filters', exact: true }).click();
	for (const [key, value] of Object.entries(values)) {
		await page.getByCSS(`#filter-${key}`).fill(value);
	}
	expect(requests).toHaveBeenLastCalledWith(empty);
	for (const key of Object.keys(values)) {
		expect(appPage.url.searchParams.get(key)).toBeNull();
	}
	await page.getByRole('button', { name: 'Apply Filters', exact: true }).click();
	await vi.waitFor(() => expect(requests).toHaveBeenLastCalledWith(values));
	await page.getByRole('button', { name: 'Filters', exact: true }).click();
	for (const [key, value] of Object.entries(values)) {
		await expect.element(page.getByCSS(`#filter-${key}`)).toHaveValue(value);
	}
	await page.getByRole('button', { name: 'Clear All', exact: true }).last().click();
	for (const key of Object.keys(values)) {
		await expect.element(page.getByCSS(`#filter-${key}`)).toHaveValue('');
	}
	expect(requests).toHaveBeenLastCalledWith(values);
	await page.getByRole('button', { name: 'Apply Filters', exact: true }).click();
	await vi.waitFor(() => expect(requests).toHaveBeenLastCalledWith(empty));
});

for (const value of ['account,unknown', 'account,account', '["one","two"]']) {
	it(`preserves scalar account, session, and host pills for ${value}`, async () => {
		setLocale('en', { reload: false });
		profile.initialize(createMockProfile([Group.ADMIN]));
		vi.spyOn(UserService, 'listUsersIncludeDeleted').mockResolvedValue([]);
		mockEgressApis();
		const keys = ['claude_account', 'claude_session', 'host'];
		for (const key of keys) {
			appPage.url.searchParams.set(key, value);
		}
		const requests = vi.fn();
		worker.use(
			http.get('*/api/egress-audit-logs', ({ request }) => {
				const params = new URL(request.url).searchParams;
				requests(keys.map((key) => params.get(key)));
				return HttpResponse.json({ items: [], total: 0 });
			})
		);

		render(EgressAuditLogsContent);
		await vi.waitFor(() => expect(requests).toHaveBeenLastCalledWith(keys.map(() => value)));
		for (const label of ['Claude account', 'Claude session', 'Destination']) {
			await expect
				.element(page.getByCSS('.filter-primary').filter({ hasText: label }))
				.toHaveTextContent(`${label} : ${value}`);
		}
		await page.getByRole('button', { name: 'Remove Claude account filter', exact: true }).click();
		await vi.waitFor(() => expect(requests).toHaveBeenLastCalledWith([null, value, value]));
	});
}

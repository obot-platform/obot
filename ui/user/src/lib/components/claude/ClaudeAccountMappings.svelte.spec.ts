import { worker } from '../../../tests/mocks/worker';
import ClaudeAccountMappings from './ClaudeAccountMappings.svelte';
import { http, HttpResponse } from 'msw';
import { expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

it('reviews email matches, preserves saved choices on reimport, and submits manual overrides', async () => {
	const craig = {
		tagged_id: 'user_01XU7fkU9xYPszwtaY87gR7z',
		uuid: 'f6bb59d7-c47a-4a58-8c35-00edf1041e0d',
		full_name: 'Craig',
		email_address: 'CRAIG@obot.ai'
	};
	const grant = {
		tagged_id: 'user_014cZgxnZBRLsqX7czjknSqk',
		uuid: '1d4297de-9478-4293-b1e3-882001da5047',
		full_name: 'Grant',
		email_address: 'grant@obot.ai'
	};
	const saved = [
		{
			organizationID: 'org',
			account: grant.tagged_id,
			uuid: grant.uuid,
			displayName: 'Grant',
			email: grant.email_address,
			userID: ''
		}
	];
	const submitted = vi.fn();
	worker.use(
		http.get('*/api/claude-organizations/org/accounts', () => HttpResponse.json({ items: saved })),
		http.get('*/api/users', () =>
			HttpResponse.json({
				items: [
					{ id: '1', displayName: 'Craig', email: 'craig@obot.ai' },
					{ id: '2', displayName: 'Grant', email: 'grant@obot.ai' }
				]
			})
		),
		http.put('*/api/claude-organizations/org/accounts', async ({ request }) => {
			const body = (await request.json()) as { items: typeof saved };
			submitted(body);
			return HttpResponse.json(body);
		})
	);
	await render(ClaudeAccountMappings, {
		organization: 'org',
		environments: [
			{ id: 'env', organizationID: 'org', environmentID: 'ccpool_test', displayName: 'Test' }
		],
		onsaved: vi.fn().mockResolvedValue(undefined)
	});
	await expect
		.element(page.getByRole('link', { name: 'https://claude.ai/api/organizations/org/members' }))
		.toHaveAttribute('href', 'https://claude.ai/api/organizations/org/members');
	await page
		.getByLabelText('Organization members JSON')
		.fill(JSON.stringify([{ account: craig }, { account: grant }]));
	await page.getByRole('button', { name: 'Preview mappings' }).click();
	await expect
		.element(page.getByRole('combobox', { name: 'Obot user for CRAIG@obot.ai', exact: true }))
		.toHaveTextContent('Craig (craig@obot.ai)');
	await expect
		.element(page.getByRole('combobox', { name: 'Obot user for grant@obot.ai', exact: true }))
		.toHaveTextContent('Grant (grant@obot.ai)');
	await page.getByRole('combobox', { name: 'Obot user for grant@obot.ai', exact: true }).click();
	await page.getByRole('button', { name: 'Not mapped', exact: true }).click();
	await page.getByRole('combobox', { name: 'Obot user for CRAIG@obot.ai', exact: true }).click();
	await page.getByRole('button', { name: 'Grant (grant@obot.ai)', exact: true }).click();
	await page.getByRole('button', { name: 'Submit mappings' }).click();
	await vi.waitFor(() =>
		expect(submitted).toHaveBeenCalledWith({
			items: expect.arrayContaining([
				expect.objectContaining({ account: craig.tagged_id, userID: '2' }),
				expect.objectContaining({ account: grant.tagged_id, userID: '' })
			])
		})
	);
	await expect.element(page.getByRole('status')).toHaveTextContent('Account mappings saved.');

	// Email now suggests Craig, but his manually saved assignment must win.
	await page.getByLabelText('Organization members JSON').fill(JSON.stringify([{ account: craig }]));
	await page.getByRole('button', { name: 'Preview mappings' }).click();
	await expect
		.element(page.getByRole('combobox', { name: 'Obot user for CRAIG@obot.ai', exact: true }))
		.toHaveTextContent('Grant (grant@obot.ai)');
	await expect.element(page.getByText('Saved mapping — absent from import')).toBeVisible();
	// An explicit duplicate assignment blocks submission.
	await page.getByRole('combobox', { name: 'Obot user for grant@obot.ai', exact: true }).click();
	await page.getByRole('button', { name: 'Grant (grant@obot.ai)', exact: true }).click();
	await expect.element(page.getByRole('button', { name: 'Submit mappings' })).toBeDisabled();
	await expect
		.element(page.getByRole('alert'))
		.toHaveTextContent(
			'Each Obot user can map to only one Claude account per organization. Resolve the duplicate selections before submitting.'
		);
});

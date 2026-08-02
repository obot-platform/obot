import { expect, test, type Page, type Route } from '@playwright/test';

const entryName = 'Static OAuth Playwright';
const clientID = 'playwright-client';
const clientSecret = 'playwright-secret';
const replacementClientID = 'playwright-client-rotated';
const replacementClientSecret = 'playwright-secret-rotated';

test('static OAuth modal tests exact credentials, expires proof, saves, and clears', async ({
	page
}) => {
	await loginAsBootstrapOwner(page);
	const entry = await createStaticOAuthEntry(page);

	let configured = false;
	let cleared = false;
	let clearAttempts = 0;
	let generation = 'initial-generation';
	let statusLoadFailures = 1;
	let saveFailures = 1;
	let attempt = 0;
	const statusReads = new Map<string, number>();
	let savedBody: Record<string, string> | undefined;
	let recoveredBody: Record<string, string> | undefined;
	let replacedBody: Record<string, string> | undefined;
	await page.route(
		`**/api/mcp-catalogs/default/entries/${entry.id}/oauth-credentials`,
		async (route) => {
			switch (route.request().method()) {
				case 'GET':
					if (statusLoadFailures-- > 0) {
						await route.fulfill({ status: 500, body: 'status unavailable' });
						return;
					}
					await json(route, {
						configured,
						clientID: configured ? clientID : undefined,
						generation: configured ? generation : undefined,
						callbackURL: 'http://127.0.0.1:18080/oauth/mcp/callback'
					});
					return;
				case 'POST':
					savedBody = route.request().postDataJSON() as Record<string, string>;
					if (saveFailures-- > 0) {
						configured = true;
						generation = 'ambiguous-generation';
						await route.fulfill({ status: 500, body: 'ambiguous save failure' });
						return;
					}
					configured = true;
					generation = 'saved-generation';
					await json(route, {
						configured: true,
						clientID,
						generation,
						callbackURL: 'http://127.0.0.1:18080/oauth/mcp/callback'
					});
					return;
				case 'PUT': {
					const replacement = route.request().postDataJSON() as Record<string, string>;
					if (replacement.clientID === clientID) {
						recoveredBody = replacement;
					} else {
						replacedBody = replacement;
					}
					generation = `generation-${replacement.clientID}`;
					await json(route, {
						configured: true,
						clientID: replacement.clientID,
						generation,
						callbackURL: 'http://127.0.0.1:18080/oauth/mcp/callback'
					});
					return;
				}
				case 'DELETE':
					clearAttempts += 1;
					expect(route.request().postDataJSON()).toEqual({ expectedGeneration: generation });
					if (clearAttempts === 1) {
						generation = 'externally-rotated-generation';
						await route.fulfill({
							status: 409,
							body: 'OAuth application changed; reload its status before clearing'
						});
						return;
					}
					configured = false;
					cleared = true;
					await route.fulfill({ status: 204, body: '' });
					return;
				default:
					await route.abort();
			}
		}
	);

	await page.route(
		`**/api/mcp-catalogs/default/entries/${entry.id}/oauth-credential-tests`,
		async (route) => {
			attempt += 1;
			const request = route.request().postDataJSON() as Record<string, string>;
			const expectedCredentials =
				attempt === 5
					? { clientID: replacementClientID, clientSecret: replacementClientSecret }
					: { clientID, clientSecret };
			expect(request).toEqual(expectedCredentials);
			await json(route, {
				testState: `test-state-${attempt}`,
				oauthURL: `${new URL(page.url()).origin}/oauth-test-popup?attempt=${attempt}`
			});
		}
	);
	await page.route(
		`**/api/mcp-catalogs/default/entries/${entry.id}/oauth-credential-tests/status`,
		async (route) => {
			const { testState } = route.request().postDataJSON() as { testState: string };
			const reads = (statusReads.get(testState) ?? 0) + 1;
			statusReads.set(testState, reads);
			const expiresAt = new Date(
				Date.now() + (testState === 'test-state-2' ? 1_500 : 5 * 60_000)
			).toISOString();
			if (reads === 1) {
				await json(route, { status: 'pending', expiresAt });
			} else if (testState === 'test-state-1') {
				await json(route, {
					status: 'failed',
					failureCategory: 'token_exchange_failed',
					expiresAt
				});
			} else {
				await json(route, {
					status: 'succeeded',
					proof: `save-proof-${testState.replace('test-state-', '')}`,
					expiresAt
				});
			}
		}
	);
	await page.route('**/oauth-test-popup?*', async (route) => {
		await route.fulfill({
			status: 200,
			contentType: 'text/html',
			body: '<title>OAuth test</title>'
		});
	});

	await page.goto('/admin/mcp-catalog');
	await expect(page.locator('#initial-loader.loaded')).toBeVisible({ timeout: 30_000 });
	await closeOpenDialogs(page);
	await clickConfigureOAuth(page);
	await expect(staticOAuthDialog(page)).not.toBeVisible();
	await openStaticOAuthModal(page);
	const dialog = staticOAuthDialog(page);
	await dialog.getByLabel('Client ID').fill(clientID);
	await dialog.locator('input[name="clientSecret"]').fill(clientSecret);
	const save = dialog.getByRole('button', { name: 'Save' });
	const testCredentials = dialog.getByRole('button', { name: 'Test Credentials' });
	await expect(save).toBeDisabled();

	const failedPopupPromise = page.waitForEvent('popup');
	await testCredentials.click();
	const failedPopup = await failedPopupPromise;
	await expect(failedPopup).toHaveURL(/oauth-test-popup\?attempt=1/);
	await page.evaluate(() => {
		window.postMessage({ type: 'oauth-success', proof: 'attacker-proof' }, '*');
	});
	await expect(save).toBeDisabled();
	await expect(dialog.getByText('The provider rejected the client credentials.')).toBeVisible();
	await expect(testCredentials).toBeEnabled();

	const expiringPopupPromise = page.waitForEvent('popup');
	await testCredentials.click();
	const expiringPopup = await expiringPopupPromise;
	await expect(expiringPopup).toHaveURL(/oauth-test-popup\?attempt=2/);
	await expect(
		dialog.getByText('Credentials tested successfully. You can save them.')
	).toBeVisible();
	await expect(save).toBeEnabled();
	await expect(dialog.getByText('The credential test expired.')).toBeVisible({ timeout: 5_000 });
	await expect(save).toBeDisabled();
	await expect(testCredentials).toBeEnabled();

	const savingPopupPromise = page.waitForEvent('popup');
	await testCredentials.click();
	const savingPopup = await savingPopupPromise;
	await expect(savingPopup).toHaveURL(/oauth-test-popup\?attempt=3/);
	await expect(save).toBeEnabled();
	await save.click();
	await expect(dialog).toBeVisible();
	const recover = dialog.getByRole('button', { name: 'Replace Credentials' });
	await expect(recover).toBeDisabled();
	await expect(dialog.getByText(/ambiguous save failure/)).toBeVisible();
	const retrySavePopupPromise = page.waitForEvent('popup');
	await testCredentials.click();
	const retrySavePopup = await retrySavePopupPromise;
	await expect(retrySavePopup).toHaveURL(/oauth-test-popup\?attempt=4/);
	await expect(recover).toBeEnabled();
	await recover.click();
	await expect(dialog).not.toBeVisible();
	expect(savedBody).toEqual({ clientID, clientSecret, proof: 'save-proof-3' });
	expect(recoveredBody).toEqual({ clientID, clientSecret, proof: 'save-proof-4' });

	await openStaticOAuthModal(page);
	await expect(dialog.getByText(/active app and user grants remain usable/)).toBeVisible();
	const replace = dialog.getByRole('button', { name: 'Replace Credentials' });
	await expect(replace).toBeDisabled();
	await dialog.getByLabel('Client ID').fill(replacementClientID);
	await dialog.locator('input[name="clientSecret"]').fill(replacementClientSecret);
	const replacementPopupPromise = page.waitForEvent('popup');
	await dialog.getByRole('button', { name: 'Test Credentials' }).click();
	const replacementPopup = await replacementPopupPromise;
	await expect(replacementPopup).toHaveURL(/oauth-test-popup\?attempt=5/);
	await expect(replace).toBeEnabled();
	await replace.click();
	await expect(dialog).not.toBeVisible();
	expect(replacedBody).toEqual({
		clientID: replacementClientID,
		clientSecret: replacementClientSecret,
		proof: 'save-proof-5'
	});

	await openStaticOAuthModal(page);
	await dialog.getByRole('button', { name: 'Clear Credentials' }).click();
	const confirm = page.getByRole('dialog').filter({ hasText: 'Confirm Delete' });
	await confirm.getByRole('button', { name: "Yes, I'm sure" }).click();
	await expect(confirm).not.toBeVisible();
	await expect(dialog).toBeVisible();
	await expect(dialog.getByText(/OAuth application changed/)).toBeVisible();
	expect(cleared).toBe(false);

	await dialog.getByRole('button', { name: 'Clear Credentials' }).click();
	await confirm.getByRole('button', { name: "Yes, I'm sure" }).click();
	await expect(confirm).not.toBeVisible();
	expect(cleared).toBe(true);
});

async function loginAsBootstrapOwner(page: Page) {
	await page.goto('/admin');
	await page.locator('input[name="bootstrap-token"]').fill('bootstrap-token');
	await page.getByRole('button', { name: 'Login' }).click();
	await expect(page).toHaveURL(/\/admin\/auth-providers/);
}

async function createStaticOAuthEntry(page: Page): Promise<{ id: string }> {
	const response = await page.request.post('/api/mcp-catalogs/default/entries', {
		data: {
			name: entryName,
			shortDescription: 'Playwright static OAuth entry',
			description: 'Playwright static OAuth entry',
			icon: '',
			runtime: 'remote',
			serverUserType: 'multiUser',
			remoteConfig: {
				fixedURL: 'http://127.0.0.1:18080/api/healthz',
				staticOAuthRequired: true
			}
		}
	});
	expect(response.ok(), await response.text()).toBeTruthy();
	return response.json() as Promise<{ id: string }>;
}

async function openStaticOAuthModal(page: Page) {
	await clickConfigureOAuth(page);
	await expect(staticOAuthDialog(page)).toBeVisible();
}

async function clickConfigureOAuth(page: Page) {
	const row = page.getByRole('row').filter({ hasText: entryName });
	await expect(row).toBeVisible({ timeout: 30_000 });
	await row.getByRole('button', { name: 'Row actions' }).click();
	await page.getByRole('button', { name: 'Configure OAuth' }).click();
}

function staticOAuthDialog(page: Page) {
	return page
		.getByRole('dialog')
		.filter({ has: page.getByRole('heading', { name: 'Configure Static OAuth' }) });
}

async function closeOpenDialogs(page: Page) {
	await page.locator('dialog[open]').evaluateAll((dialogs) => {
		for (const dialog of dialogs) {
			(dialog as HTMLDialogElement).close();
		}
	});
	await expect(page.locator('dialog[open]')).toHaveCount(0);
}

async function json(route: Route, body: unknown) {
	await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(body) });
}

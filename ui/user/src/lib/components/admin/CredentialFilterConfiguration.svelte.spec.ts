import CredentialFilterConfiguration from './CredentialFilterConfiguration.svelte';
import { credentialKeys } from './credentialPolicy';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function config(values: Record<string, string> = {}) {
	return credentialKeys.map((key) => ({
		key,
		name: key,
		description: '',
		required: false,
		sensitive: false,
		value: values[key] || ''
	}));
}

describe('credential filter configuration', () => {
	it('clears the selected rule when the search changes', async () => {
		render(CredentialFilterConfiguration, { config: config() });
		await page.getByRole('button', { name: '+ Add override', exact: true }).click();
		const search = page.getByLabelText('Search provider, credential type, or rule ID', {
			exact: true
		});
		const add = page.getByRole('button', { name: 'Add override', exact: true });
		await search.fill('np.github.1');
		await page
			.getByRole('radio', { name: 'GitHub Personal Access Token np.github.1', exact: true })
			.click();
		await expect.element(add).toBeEnabled();
		await search.fill('no-such-rule');
		await expect.element(page.getByText('No credential types found')).toBeVisible();
		await expect.element(add).toBeDisabled();
		await search.fill('np.slack.2');
		await expect.element(add).toBeDisabled();
		await page.getByRole('radio', { name: 'Slack Bot Token np.slack.2', exact: true }).click();
		await add.click();
		await expect
			.element(page.getByRole('button', { name: 'Remove Slack Bot Token' }))
			.toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Remove GitHub Personal Access Token' }))
			.not.toBeInTheDocument();
	});
	it.each(['Redact', 'Allow'])('sets %s when the default field is missing', async (action) => {
		render(CredentialFilterConfiguration, { config: [] });
		const defaultAction = page.getByRole('combobox', { name: 'Default action', exact: true });
		await defaultAction.click();
		await page.getByRole('button', { name: action, exact: true }).click();
		await expect.element(defaultAction).toHaveTextContent(action);
	});
	it('changes the default without discarding an invalid saved override', async () => {
		render(CredentialFilterConfiguration, {
			config: config({ CREDENTIAL_ALLOW_RULES: 'removed.rule' })
		});
		const defaultAction = page.getByRole('combobox', { name: 'Default action', exact: true });
		await defaultAction.click();
		await page.getByRole('button', { name: 'Redact', exact: true }).click();
		await expect.element(defaultAction).toHaveTextContent('Redact');
		await expect
			.element(page.getByRole('alert'))
			.toHaveTextContent(
				'Unsupported credential rule removed.rule. Remove this override or use a compatible filter version.'
			);
		await expect.element(page.getByRole('button', { name: 'Remove removed.rule' })).toBeVisible();
	});
	it('starts with Block and no overrides', async () => {
		render(CredentialFilterConfiguration, { config: config() });
		await expect
			.element(page.getByLabelText('Default action', { exact: true }))
			.toHaveTextContent('Block');
		await expect
			.element(page.getByText('No overrides. All credential types use the default action.'))
			.toBeVisible();
	});
	it('searches rules, preserves explicit overrides, prevents duplicates, and removes overrides', async () => {
		render(CredentialFilterConfiguration, { config: config() });
		await page.getByRole('button', { name: '+ Add override', exact: true }).click();
		const search = page.getByLabelText('Search provider, credential type, or rule ID', {
			exact: true
		});
		await search.fill('np.github.1');
		await page
			.getByRole('radio', { name: 'GitHub Personal Access Token np.github.1', exact: true })
			.click();
		await page.getByRole('combobox', { name: 'Action', exact: true }).click();
		await page.getByRole('button', { name: 'Redact', exact: true }).click();
		await page.getByRole('button', { name: 'Add override', exact: true }).click();
		await expect
			.element(page.getByLabelText('GitHub Personal Access Token', { exact: true }))
			.toHaveTextContent('Redact');
		await page.getByRole('combobox', { name: 'Default action', exact: true }).click();
		await page.getByRole('button', { name: 'Allow', exact: true }).click();
		await expect
			.element(page.getByLabelText('GitHub Personal Access Token', { exact: true }))
			.toHaveTextContent('Redact');
		await page.getByRole('button', { name: '+ Add override', exact: true }).click();
		await expect.element(page.getByLabelText('Action', { exact: true })).toHaveTextContent('Allow');
		await search.fill('GitHub Personal Access Token');
		await expect
			.element(
				page.getByRole('radio', {
					name: 'GitHub Personal Access Token np.github.1 — Already added',
					exact: true
				})
			)
			.toBeDisabled();
		await search.fill('Slack');
		await expect
			.element(page.getByRole('radio', { name: /Slack Bot Token np.slack.2/ }))
			.toBeVisible();
		await search.fill('no-such-credential');
		await expect.element(page.getByText('No credential types found')).toBeVisible();
		await page.getByRole('button', { name: 'Cancel', exact: true }).click();
		await page.getByRole('button', { name: 'Remove GitHub Personal Access Token' }).click();
		await expect
			.element(page.getByText('No overrides. All credential types use the default action.'))
			.toBeVisible();
	});
	it('shows unknown saved IDs and lets the user remove them explicitly', async () => {
		render(CredentialFilterConfiguration, {
			config: config({ CREDENTIAL_ALLOW_RULES: 'removed.rule' })
		});
		await expect
			.element(page.getByRole('alert'))
			.toHaveTextContent(
				'Unsupported credential rule removed.rule. Remove this override or use a compatible filter version.'
			);
		await page.getByRole('button', { name: 'Remove removed.rule' }).click();
		await expect.element(page.getByRole('alert')).not.toBeInTheDocument();
	});
	it('disables editing in read-only mode', async () => {
		render(CredentialFilterConfiguration, { config: config(), readonly: true });
		await expect
			.element(page.getByRole('combobox', { name: 'Default action', exact: true }))
			.toHaveAttribute('tabindex', '-1');
		await expect
			.element(page.getByRole('button', { name: '+ Add override', exact: true }))
			.toBeDisabled();
	});
});

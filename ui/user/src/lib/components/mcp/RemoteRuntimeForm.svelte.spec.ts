import type {
	LegacyRemoteCatalogConfigAdmin,
	MCPCatalogEntryFieldManifest,
	RemoteRuntimeConfigAdmin
} from '$lib/services/admin/types';
import RemoteRuntimeForm from './RemoteRuntimeForm.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function header(
	overrides: Partial<MCPCatalogEntryFieldManifest> = {}
): MCPCatalogEntryFieldManifest {
	return {
		key: 'Authorization',
		name: '',
		description: '',
		value: '',
		required: false,
		sensitive: false,
		...overrides
	};
}

const optionalUserSuppliedHeader = header({
	name: 'Context7 API Key',
	description: 'Optional API key for higher limits and private sources',
	required: false,
	sensitive: true,
	prefix: 'Bearer '
});

function renderServerForm(headers: MCPCatalogEntryFieldManifest[]) {
	const config: RemoteRuntimeConfigAdmin = { url: 'https://mcp.example.com/mcp', headers };
	return render(RemoteRuntimeForm, { config, variant: 'server' });
}

function renderCatalogForm(headers: MCPCatalogEntryFieldManifest[], readonly = false) {
	// A hostname keeps the advanced section -- and with it the Headers block -- expanded even
	// when the entry has no headers yet.
	const config = $state<LegacyRemoteCatalogConfigAdmin>({ hostname: 'mcp.example.com', headers });
	return render(RemoteRuntimeForm, { config, variant: 'catalog', readonly });
}

const valueTypeSelect = () => page.getByCSS('#header-value-type-0');
const prefixInput = () => page.getByLabelText('Value Prefix');
const requiredToggle = () => page.getByRole('switch', { name: 'Required' });

describe('RemoteRuntimeForm.svelte headers', () => {
	// Every header on a deployed server takes an admin-supplied value, so each one gets a value
	// input -- masked when the header is sensitive -- no matter how the manifest describes it.
	describe('deployed server', () => {
		it.each([
			{
				name: 'a required header awaiting a value',
				overrides: { required: true },
				value: '',
				type: 'text'
			},
			{
				name: 'a required header with a revealed value',
				overrides: { required: true, value: 'secret' },
				value: 'secret',
				type: 'text'
			},
			{
				name: 'a required sensitive header',
				overrides: { required: true, sensitive: true, value: 'secret' },
				value: 'secret',
				type: 'password'
			},
			{
				name: 'an optional sensitive header',
				overrides: { sensitive: true, value: 'secret' },
				value: 'secret',
				type: 'password'
			},
			{
				name: 'an optional plain header',
				overrides: { value: 'acme' },
				value: 'acme',
				type: 'text'
			}
		])('shows $name in a $type value input', async ({ overrides, value, type }) => {
			await renderServerForm([header({ name: 'API Key', prefix: 'Bearer ', ...overrides })]);

			const input = page.getByLabelText('Value');
			await expect.element(input).toHaveValue(value);
			await expect.element(input).toHaveAttribute('type', type);
		});
	});

	describe('catalog entry', () => {
		it('treats an optional header without a value as user-supplied', async () => {
			await renderCatalogForm([optionalUserSuppliedHeader]);

			await expect.element(valueTypeSelect()).toHaveTextContent('User-Supplied');
			await expect.element(prefixInput()).toHaveValue('Bearer ');
			await expect.element(requiredToggle()).not.toBeChecked();
		});

		it('keeps a header with a value static', async () => {
			await renderCatalogForm([header({ value: 'static-token' })]);

			await expect.element(valueTypeSelect()).toHaveTextContent('Static');
		});

		it('does not let the value type be changed on a read-only entry', async () => {
			await renderCatalogForm([optionalUserSuppliedHeader], true);

			await expect.element(valueTypeSelect()).toHaveAttribute('tabindex', '-1');
			await expect
				.element(page.getByRole('button', { name: 'Static', exact: true }))
				.not.toBeInTheDocument();
		});

		it('keeps a user-supplied header user-supplied after Required is turned off', async () => {
			await renderCatalogForm([header({ name: 'API Key', required: true, prefix: 'Bearer ' })]);

			await requiredToggle().click();

			await expect.element(requiredToggle()).not.toBeChecked();
			await expect.element(valueTypeSelect()).toHaveTextContent('User-Supplied');
			await expect.element(prefixInput()).toHaveValue('Bearer ');
		});

		it('keeps a newly added header static until another value type is picked', async () => {
			await renderCatalogForm([]);

			await page.getByRole('button', { name: 'Header', exact: true }).click();

			await expect.element(valueTypeSelect()).toHaveTextContent('Static');
			await expect.element(prefixInput()).not.toBeInTheDocument();
		});

		it('clears the prefix when a user-supplied header is switched to static', async () => {
			await renderCatalogForm([optionalUserSuppliedHeader]);

			await valueTypeSelect().click();
			await page.getByRole('button', { name: 'Static', exact: true }).click();

			await expect.element(valueTypeSelect()).toHaveTextContent('Static');
			await expect.element(prefixInput()).not.toBeInTheDocument();
		});
	});
});

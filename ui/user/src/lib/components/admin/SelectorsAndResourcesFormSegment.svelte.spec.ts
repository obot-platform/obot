import type {
	MCPFilterLocalAgentEvent,
	MCPFilterResource,
	MCPFilterWebhookSelector
} from '$lib/services';
import { mcpServersAndEntries } from '$lib/stores';
import { createMCPCatalogEntry, createMCPCatalogServer } from '../../../tests/helpers/mcp';
import SelectorsAndResourcesFormSegment from './SelectorsAndResourcesFormSegment.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

type Form = {
	localAgentEvents: MCPFilterLocalAgentEvent[];
	resources: MCPFilterResource[];
	selectors: MCPFilterWebhookSelector[];
};

function renderForm(form: Form, readonly = false) {
	return render(SelectorsAndResourcesFormSegment, { form, readonly });
}

describe('SelectorsAndResourcesFormSegment.svelte', () => {
	it('adds All Devices with every local agent event selected as a wildcard', async () => {
		const form: Form = {
			localAgentEvents: [],
			resources: [{ type: 'mcpCatalog', id: 'default' }],
			selectors: []
		};
		await renderForm(form);

		await page.getByLabelText('All Devices', { exact: true }).click();

		await expect.element(page.getByLabelText('User prompts', { exact: true })).toBeChecked();
		await expect.element(page.getByLabelText('Tool call arguments', { exact: true })).toBeChecked();
		await expect.element(page.getByLabelText('Tool responses', { exact: true })).toBeChecked();
		expect(form.resources).toContainEqual({ type: 'deviceSelector', id: '*' });
		expect(form.localAgentEvents).toEqual(['*']);
	});

	it('edits explicit event selections and collapses all events to the wildcard', async () => {
		const form: Form = {
			localAgentEvents: ['userPrompt', 'toolCallArguments'],
			resources: [
				{ type: 'selector', id: '*' },
				{ type: 'deviceSelector', id: '*' }
			],
			selectors: []
		};
		await renderForm(form);

		await expect.element(page.getByLabelText('All Devices', { exact: true })).toBeChecked();
		await expect.element(page.getByLabelText('Tool responses', { exact: true })).not.toBeChecked();
		await page.getByLabelText('Tool responses', { exact: true }).click();

		expect(form.localAgentEvents).toEqual(['*']);

		await page.getByRole('button', { name: 'Remove MCP Target', exact: true }).click();
		await expect
			.element(page.getByRole('button', { name: 'Remove MCP Target', exact: true }))
			.not.toBeInTheDocument();
		await expect.element(page.getByLabelText('All Devices', { exact: true })).toBeChecked();
	});

	it('removes resolved MCP servers and catalog entries', async () => {
		const previousStore = mcpServersAndEntries.current;
		const catalogEntry = createMCPCatalogEntry({
			id: 'mcpcatentry-test',
			name: 'Catalog Entry'
		});
		const server = createMCPCatalogServer({
			id: 'mcpserver-test',
			name: 'MCP Server',
			userID: 'user-test'
		});
		mcpServersAndEntries.current = {
			...previousStore,
			entries: [catalogEntry],
			servers: [server]
		};

		try {
			const form: Form = {
				localAgentEvents: [],
				resources: [
					{ type: 'mcpServer', id: server.id },
					{ type: 'mcpServerCatalogEntry', id: catalogEntry.id }
				],
				selectors: []
			};
			await renderForm(form);

			const serverRow = page.getByRole('row').filter({ hasText: 'MCP Server' });
			await serverRow.getByRole('button', { name: 'Remove MCP Target', exact: true }).click();
			expect(form.resources).toEqual([{ type: 'mcpServerCatalogEntry', id: catalogEntry.id }]);

			const catalogEntryRow = page.getByRole('row').filter({ hasText: 'Catalog Entry' });
			await catalogEntryRow.getByRole('button', { name: 'Remove MCP Target', exact: true }).click();
			expect(form.resources).toEqual([]);
		} finally {
			mcpServersAndEntries.current = previousStore;
		}
	});

	it('shows device targeting read-only without edit controls', async () => {
		const form: Form = {
			localAgentEvents: ['toolResponse'],
			resources: [{ type: 'deviceSelector', id: '*' }],
			selectors: []
		};
		await renderForm(form, true);

		await expect.element(page.getByLabelText('All Devices', { exact: true })).toBeDisabled();
		await expect.element(page.getByLabelText('User prompts', { exact: true })).toBeDisabled();
		await expect
			.element(page.getByLabelText('Tool call arguments', { exact: true }))
			.toBeDisabled();
		await expect.element(page.getByLabelText('Tool responses', { exact: true })).toBeDisabled();
		await expect
			.element(page.getByRole('button', { name: 'Add MCP Target', exact: true }))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: 'Add Selector', exact: true }))
			.not.toBeInTheDocument();
	});
});

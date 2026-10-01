import { createMCPCatalogServer } from '../../../tests/helpers/mcp';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import EditExistingDeployment from './EditExistingDeployment.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

describe('EditExistingDeployment.svelte', () => {
	it('does not ask for static configuration stored by the catalog entry', async () => {
		await preparePageData({ profile: createMockProfile([]) });
		const server = createMCPCatalogServer({
			id: 'ms1static',
			name: 'Static server',
			userID: 'user-1',
			env: [
				{
					key: 'API_TOKEN',
					name: 'API token',
					description: '',
					required: true,
					sensitive: true,
					value: '',
					static: true
				},
				{
					key: 'REGION',
					name: 'Region',
					description: '',
					required: true,
					sensitive: false,
					value: ''
				}
			]
		});
		worker.use(
			http.post(`/api/mcp-catalogs/${server.mcpCatalogID}/servers/${server.id}/reveal`, () =>
				HttpResponse.json({ REGION: 'us' })
			)
		);

		const result = await render(EditExistingDeployment, {});
		await result.component.edit({ server });

		await expect.element(page.getByText('Region', { exact: true })).toBeVisible();
		await expect.element(page.getByText('API token', { exact: true })).not.toBeInTheDocument();
	});
});

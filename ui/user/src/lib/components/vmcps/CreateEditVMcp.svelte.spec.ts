import type { VMCPManifest } from '$lib/services';
import { catalogEntryToVMCPComponent } from '$lib/services/vmcps/utils';
import { createMCPCatalogEntry, createVMCP } from '../../../tests/helpers/mcp';
import { preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import CreateEditVMcp from './CreateEditVMcp.svelte';
import { http, HttpResponse } from 'msw';
import { expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

it('sends required configuration as fixed when creating a vMCP', async () => {
	await preparePageData();
	const entry = createMCPCatalogEntry({
		id: 'entry-configurable',
		name: 'Configurable server',
		manifest: {
			config: [
				{
					key: 'TOKEN',
					name: 'Token',
					description: '',
					required: true,
					sensitive: true,
					value: '',
					usage: 'env'
				},
				{
					key: 'REGION',
					name: 'Region',
					description: '',
					required: true,
					sensitive: false,
					value: 'us-east-1',
					usage: 'header'
				},
				{
					key: 'OPTIONAL',
					name: 'Optional',
					description: '',
					required: false,
					sensitive: false,
					value: '',
					usage: 'env'
				}
			]
		}
	});
	const create = vi.fn();
	worker.use(
		http.post('/api/vmcps', async ({ request }) => {
			const manifest = (await request.json()) as VMCPManifest;
			create(manifest);
			return HttpResponse.json(createVMCP(manifest));
		})
	);
	const result = await render(CreateEditVMcp);
	result.component.openCreate([catalogEntryToVMCPComponent(entry)]);
	await page.getByRole('button', { name: 'Create', exact: true }).click();
	await vi.waitFor(() => expect(create).toHaveBeenCalledOnce());
	expect(create.mock.calls[0][0].components[0].configuration).toEqual([
		{ key: 'TOKEN', policy: 'fixed', value: '' },
		{ key: 'REGION', policy: 'fixed', value: 'us-east-1' }
	]);
});

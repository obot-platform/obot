import { Group, type VMCPManifest } from '$lib/services';
import { catalogEntryToVMCPComponent } from '$lib/services/vmcps/utils';
import { errors } from '$lib/stores';
import { createMCPCatalogEntry, createVMCP } from '../../../tests/helpers/mcp';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
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

const overPlan =
	"There isn't room on the plan's hosted MCP server memory for this. Free some up, or ask an owner to change the plan.";

async function createFailing(status: number, body: string) {
	worker.use(http.post('/api/vmcps', () => new HttpResponse(body, { status })));
	errors.items = [];
	const result = await render(CreateEditVMcp);
	result.component.openCreate();
	await page.getByRole('textbox', { name: 'Name' }).fill('Search');
	await page.getByRole('textbox', { name: 'Description' }).fill('Searches the web');
	await page.getByRole('button', { name: 'Create', exact: true }).click();
}

it('shows why a create failed in the dialog instead of a toast', async () => {
	await preparePageData({ profile: createMockProfile([Group.ADMIN]) });

	await createFailing(402, overPlan);

	const alert = page.getByRole('alert');
	await expect.element(alert).toHaveTextContent(`Failed to create vMCP. ${overPlan} View Plan`);
	await expect
		.element(alert.getByRole('link', { name: 'View Plan', exact: true }))
		.toHaveAttribute('href', '/admin/platform?view=license');
	// The dialog stays open, so the form can be fixed and sent again.
	await expect.element(page.getByRole('button', { name: 'Create', exact: true })).toBeVisible();
	expect(errors.items).toHaveLength(0);
});

it("doesn't offer the plan to users who can't view it", async () => {
	await preparePageData({ profile: createMockProfile([Group.USER]) });

	await createFailing(402, overPlan);

	await expect
		.element(page.getByRole('alert'))
		.toHaveTextContent(`Failed to create vMCP. ${overPlan}`);
	await expect
		.element(page.getByRole('link', { name: 'View Plan', exact: true }))
		.not.toBeInTheDocument();
});

it('shows the reason for other failures too', async () => {
	await preparePageData();

	await createFailing(400, 'component "search" needs an image');

	const alert = page.getByRole('alert');
	await expect
		.element(alert)
		.toHaveTextContent('Failed to create vMCP. component "search" needs an image');
	await expect
		.element(page.getByRole('link', { name: 'View Plan', exact: true }))
		.not.toBeInTheDocument();
});

it('refuses a second create while one is saving', async () => {
	await preparePageData({ profile: createMockProfile([Group.ADMIN]) });
	let respond: (() => void) | undefined;
	worker.use(
		http.post('/api/vmcps', async () => {
			await new Promise<void>((resolve) => (respond = resolve));
			return HttpResponse.json(createVMCP({ id: 'vmcp-1', displayName: 'Search' }));
		})
	);
	const result = await render(CreateEditVMcp);
	const draft = { displayName: 'Search', description: '', toolSearch: false, components: [] };

	const first = result.component.saveCreate(draft);
	await expect(result.component.saveCreate(draft)).rejects.toThrow('Failed to create vMCP');
	await vi.waitFor(() => expect(respond).toBeDefined());
	respond?.();
	await expect(first).resolves.toBeUndefined();
});

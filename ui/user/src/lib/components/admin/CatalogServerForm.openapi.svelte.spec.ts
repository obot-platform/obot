import { CATALOG_SERVER_FIELD_IDS } from '$lib/constants';
import type { MCPCatalogEntry } from '$lib/services';
import { version } from '$lib/stores';
import { createMCPCatalogEntryResponse, getVersionResponse } from '../../../tests/mocks/data';
import { worker } from '../../../tests/mocks/worker';
import CatalogServerForm from './CatalogServerForm.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page, userEvent } from 'vitest/browser';

const schema = {
	openapi: '3.1.0',
	info: { title: 'Example', version: '1' },
	servers: [{ url: 'https://example.com/api' }],
	paths: {}
};
const sourceURL = 'https://example.com/openapi.json';
const suggestedHeader = {
	key: 'Authorization',
	name: 'API key',
	usage: 'header',
	required: true,
	sensitive: true,
	prefix: 'Bearer '
};

beforeEach(() => {
	worker.use(
		http.get('/api/mcp-catalogs/default/access-control-rules', () => HttpResponse.json([]))
	);
});

afterEach(() => version.initialize(getVersionResponse));

it('does not offer OpenAPI as a hosted runtime', async () => {
	await render(CatalogServerForm, { id: 'test', type: 'hosted' });
	await page.getByCSS('#runtime-selector').click();
	await expect.element(page.getByRole('button', { name: 'NPX', exact: true })).toBeVisible();
	await expect.element(page.getByRole('button', { name: 'UVX', exact: true })).toBeVisible();
	await expect
		.element(page.getByRole('button', { name: 'Containerized', exact: true }))
		.toBeVisible();
	await expect
		.element(page.getByRole('button', { name: 'OpenAPI', exact: true }))
		.not.toBeInTheDocument();
});

async function openForm(entity: 'catalog' | 'workspace' = 'catalog') {
	return await render(CatalogServerForm, { id: 'test', entity, type: 'openapi' });
}

function mockImport(collection = 'mcp-catalogs') {
	const request = vi.fn();
	worker.use(
		http.post(`/api/${collection}/test/openapi/import`, async ({ request: req }) => {
			request(await req.json());
			return HttpResponse.json({
				schema,
				baseURL: 'https://example.com/api',
				suggestedHeaders: [suggestedHeader],
				suggestedMetadata: { name: 'Example' }
			});
		})
	);
	return request;
}

it('imports before saving and keeps the snapshot and header prefix', async () => {
	const imported = mockImport();
	const saved = vi.fn();
	worker.use(
		http.post('/api/mcp-catalogs/test/entries', async ({ request }) => {
			saved(await request.json());
			return HttpResponse.json(createMCPCatalogEntryResponse);
		})
	);
	await openForm();
	await expect.element(page.getByRole('button', { name: 'Save', exact: true })).toBeVisible();
	await page.getByLabelText('Schema URL', { exact: true }).fill(sourceURL);
	await page.getByRole('button', { name: 'Import schema', exact: true }).click();
	await expect
		.element(page.getByRole('status'))
		.toHaveTextContent('Schema imported. The snapshot will be saved with this entry.');
	expect(imported).toHaveBeenCalledWith({ source: { url: sourceURL } });
	await expect.element(page.getByLabelText('Value Prefix')).toHaveValue('Bearer ');
	await page
		.getByLabelText('API request base URL', { exact: false })
		.fill('https://override.example.com');
	await page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.name}`).fill('Example API');
	await page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.shortDescription}`).fill('An example API');
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await vi.waitFor(() => expect(saved).toHaveBeenCalled());
	expect(saved.mock.calls[0][0]).toMatchObject({
		runtime: 'openapi',
		openAPIConfig: {
			source: { url: sourceURL },
			schema,
			baseURL: 'https://override.example.com'
		},
		config: [suggestedHeader]
	});
});

it('saves OpenAPI egress domains when network policy is enabled', async () => {
	version.initialize({ ...getVersionResponse, mcpNetworkPolicyEnabled: true });
	mockImport();
	const saved = vi.fn();
	worker.use(
		http.post('/api/mcp-catalogs/test/entries', async ({ request }) => {
			saved(await request.json());
			return HttpResponse.json(createMCPCatalogEntryResponse);
		})
	);
	await openForm();
	await page.getByLabelText('Schema URL', { exact: true }).fill(sourceURL);
	await page.getByRole('button', { name: 'Import schema', exact: true }).click();
	await expect.element(page.getByText('Egress Domains')).toBeVisible();
	const domains = page.getByPlaceholder('hit "Enter" to insert');
	await domains.fill(' api.example.com ');
	await userEvent.keyboard('{Enter}');
	await page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.shortDescription}`).fill('An example API');
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await vi.waitFor(() => expect(saved).toHaveBeenCalled());
	expect(saved.mock.calls[0][0].openAPIConfig).toMatchObject({
		egressDomains: ['api.example.com']
	});
});

it('allows all egress explicitly when deny all is the default', async () => {
	version.initialize({
		...getVersionResponse,
		mcpNetworkPolicyEnabled: true,
		mcpDefaultDenyAllEgress: true
	});
	mockImport();
	const saved = vi.fn();
	worker.use(
		http.post('/api/mcp-catalogs/test/entries', async ({ request }) => {
			saved(await request.json());
			return HttpResponse.json(createMCPCatalogEntryResponse);
		})
	);
	await openForm();
	await page.getByLabelText('Schema URL', { exact: true }).fill(sourceURL);
	await page.getByRole('button', { name: 'Import schema', exact: true }).click();
	await page.getByRole('switch', { name: 'Allow all egress' }).click();
	await page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.shortDescription}`).fill('An example API');
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await vi.waitFor(() => expect(saved).toHaveBeenCalled());
	expect(saved.mock.calls[0][0].openAPIConfig).toMatchObject({
		egressDomains: [],
		denyAllEgress: false
	});
});

it('prefills new entry metadata and displays the schema base URL only as a placeholder', async () => {
	const importedSchema = {
		...schema,
		info: {
			title: 'Raw schema title (use API suggestions instead)',
			version: '1',
			summary: 'Find documentation',
			description: 'Full API description',
			'x-logo': { url: 'https://example.com/logo.svg' }
		}
	};
	worker.use(
		http.post('/api/mcp-catalogs/test/openapi/import', () =>
			HttpResponse.json({
				schema: importedSchema,
				suggestedMetadata: {
					name: 'Context API',
					shortDescription: 'Find documentation',
					description: 'Full API description',
					icon: 'https://example.com/logo.svg'
				},
				baseURL: 'https://example.com/api',
				suggestedHeaders: []
			})
		)
	);
	const saved = vi.fn();
	worker.use(
		http.post('/api/mcp-catalogs/test/entries', async ({ request }) => {
			saved(await request.json());
			return HttpResponse.json(createMCPCatalogEntryResponse);
		})
	);
	await openForm();
	await expect.element(page.getByRole('dialog')).not.toBeInTheDocument();
	await expect
		.element(page.getByLabelText('API request base URL', { exact: false }))
		.not.toBeInTheDocument();
	await expect
		.element(page.getByText('Where tool calls are sent.', { exact: false }))
		.not.toBeInTheDocument();
	await expect.element(page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.name}`)).toBeVisible();
	await page.getByLabelText('Schema URL', { exact: true }).fill(sourceURL);
	await page.getByRole('button', { name: 'Import schema', exact: true }).click();
	await expect
		.element(page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.name}`))
		.toHaveValue('Context API');
	await expect
		.element(page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.shortDescription}`))
		.toHaveValue('Find documentation');
	await expect
		.element(page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.icon}`))
		.toHaveValue('https://example.com/logo.svg');
	await expect
		.element(page.getByLabelText('API request base URL', { exact: false }))
		.toHaveValue('');
	await expect
		.element(page.getByLabelText('API request base URL', { exact: false }))
		.toHaveAttribute('placeholder', 'https://example.com/api');
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await vi.waitFor(() => expect(saved).toHaveBeenCalled());
	expect(saved.mock.calls[0][0]).toMatchObject({
		description: 'Full API description',
		openAPIConfig: { schema: importedSchema }
	});
	expect(saved.mock.calls[0][0].openAPIConfig.baseURL).toBeUndefined();
});

it('keeps entered details visible and confirms suggestions after inline import', async () => {
	mockImport();
	await openForm();
	await page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.name}`).fill('My server');
	await expect.element(page.getByRole('dialog')).not.toBeInTheDocument();
	await expect.element(page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.name}`)).toHaveValue('My server');
	await page.getByLabelText('Schema URL', { exact: true }).fill(sourceURL);
	await page.getByRole('button', { name: 'Import schema', exact: true }).click();
	await expect
		.element(page.getByRole('heading', { name: 'Use details from schema?' }))
		.toBeVisible();
	await expect.element(page.getByRole('checkbox', { name: 'Name', exact: true })).not.toBeChecked();
	await page.getByRole('button', { name: 'Keep current values', exact: true }).click();
	await expect.element(page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.name}`)).toHaveValue('My server');
});

it('only replaces selected metadata fields after importing a replacement', async () => {
	const nextSchema = {
		...schema,
		info: {
			title: 'New name',
			version: '2',
			summary: 'New summary',
			description: 'New description'
		}
	};
	worker.use(
		http.post('/api/mcp-catalogs/test/openapi/import', () =>
			HttpResponse.json({
				schema: nextSchema,
				suggestedMetadata: {
					name: 'New name',
					shortDescription: 'New summary',
					description: 'New description'
				},
				baseURL: 'https://example.com/api',
				suggestedHeaders: []
			})
		)
	);
	const entry = {
		...createMCPCatalogEntryResponse,
		manifest: {
			...createMCPCatalogEntryResponse.manifest,
			name: 'My name',
			shortDescription: 'My summary',
			description: 'My description',
			runtime: 'openapi',
			openAPIConfig: { source: { url: sourceURL }, schema, baseURL: 'https://override.example.com' }
		}
	} as MCPCatalogEntry;
	await render(CatalogServerForm, { id: 'test', entity: 'catalog', entry });
	await page.getByRole('button', { name: 'Replace schema', exact: true }).click();
	await page.getByRole('button', { name: 'Import schema', exact: true }).click();
	await expect
		.element(page.getByRole('heading', { name: 'Use details from schema?' }))
		.toBeVisible();
	await expect.element(page.getByRole('checkbox', { name: 'Name', exact: true })).not.toBeChecked();
	await expect.element(page.getByText('Current: My name', { exact: true })).toBeVisible();
	await page.getByRole('checkbox', { name: 'Short description', exact: true }).click();
	await page.getByRole('button', { name: 'Apply selected', exact: true }).click();
	await expect.element(page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.name}`)).toHaveValue('My name');
	await expect
		.element(page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.shortDescription}`))
		.toHaveValue('New summary');
	await expect
		.element(page.getByLabelText('API request base URL', { exact: false }))
		.toHaveValue('https://override.example.com');
	expect(entry.manifest.openAPIConfig?.schema).toEqual(schema);
});

it('requires a base URL inline after importing a schema with only a relative server', async () => {
	const saved = vi.fn();
	worker.use(
		http.post('/api/mcp-catalogs/test/openapi/import', () =>
			HttpResponse.json({
				schema: { ...schema, servers: [{ url: '/api/v3' }] },
				baseURL: '',
				suggestedHeaders: [],
				suggestedMetadata: { name: 'Example', shortDescription: 'Example API' }
			})
		),
		http.post('/api/mcp-catalogs/test/entries', async ({ request }) => {
			saved(await request.json());
			return HttpResponse.json(createMCPCatalogEntryResponse);
		})
	);
	await openForm();
	await page.getByLabelText('Schema URL', { exact: true }).fill(sourceURL);
	await page.getByRole('button', { name: 'Import schema', exact: true }).click();
	const baseURL = page.getByLabelText('API request base URL', { exact: false });
	await expect.element(baseURL).toBeRequired();
	await expect.element(baseURL).not.toHaveAttribute('aria-invalid', 'true');
	await expect.element(baseURL).not.toHaveClass('error');
	await expect
		.element(page.getByText('This schema has no usable absolute server URL.', { exact: false }))
		.toBeVisible();
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await expect.element(baseURL).toHaveFocus();
	await expect.element(baseURL).toHaveAttribute('aria-invalid', 'true');
	await expect.element(baseURL).toHaveClass('error');
	expect(saved).not.toHaveBeenCalled();
	await baseURL.fill('https://petstore3.swagger.io/api/v3');
	await expect.element(baseURL).not.toHaveAttribute('aria-invalid', 'true');
	await expect
		.element(page.getByText('This schema has no usable absolute server URL.', { exact: false }))
		.not.toBeInTheDocument();
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await vi.waitFor(() =>
		expect(saved).toHaveBeenCalledWith(
			expect.objectContaining({
				openAPIConfig: expect.objectContaining({ baseURL: 'https://petstore3.swagger.io/api/v3' })
			})
		)
	);
});

it('shows an applied description in the already-mounted editor', async () => {
	const saved = vi.fn();
	worker.use(
		http.post('/api/mcp-catalogs/test/entries', async ({ request }) => {
			saved(await request.json());
			return HttpResponse.json(createMCPCatalogEntryResponse);
		})
	);
	worker.use(
		http.post('/api/mcp-catalogs/test/openapi/import', () =>
			HttpResponse.json({
				schema,
				baseURL: 'https://example.com/api',
				suggestedHeaders: [],
				suggestedMetadata: { name: 'Example', description: 'Imported API description' }
			})
		)
	);
	await openForm();
	await page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.name}`).fill('My API');
	await page.getByLabelText('Schema URL', { exact: true }).fill(sourceURL);
	await page.getByRole('button', { name: 'Import schema', exact: true }).click();
	await expect
		.element(page.getByRole('checkbox', { name: 'Description', exact: true }))
		.toBeChecked();
	await page.getByRole('checkbox', { name: 'Name', exact: true }).click();
	await page.getByRole('button', { name: 'Apply selected', exact: true }).click();
	await expect.element(page.getByRole('dialog')).not.toBeInTheDocument();
	await expect.element(page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.name}`)).toHaveValue('Example');
	await expect
		.element(page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.description}-container .cm-content`))
		.toHaveTextContent('Imported API description');
	await page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.shortDescription}`).fill('API summary');
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await vi.waitFor(() =>
		expect(saved).toHaveBeenCalledWith(
			expect.objectContaining({ name: 'Example', description: 'Imported API description' })
		)
	);
});

it('uses the workspace import endpoint and stages source changes without losing the current snapshot', async () => {
	const imported = mockImport('workspaces');
	await openForm('workspace');
	await page.getByLabelText('Schema URL', { exact: true }).fill(sourceURL);
	await page.getByRole('button', { name: 'Import schema', exact: true }).click();
	await expect
		.element(page.getByRole('status'))
		.toHaveTextContent('Schema imported. The snapshot will be saved with this entry.');
	expect(imported).toHaveBeenCalledOnce();
	await page.getByRole('button', { name: 'Replace schema', exact: true }).click();
	await page.getByLabelText('Schema URL', { exact: true }).fill('https://example.com/new.json');
	await page.getByRole('button', { name: 'Cancel import', exact: true }).click();
	await expect.element(page.getByText(sourceURL, { exact: true })).toBeVisible();
	await expect
		.element(page.getByRole('status'))
		.toHaveTextContent('Schema imported. The snapshot will be saved with this entry.');
});

it('shows import errors and allows retry without creating an entry', async () => {
	worker.use(
		http.post(
			'/api/mcp-catalogs/test/openapi/import',
			() => new HttpResponse('schema must be a JSON or YAML document', { status: 400 })
		)
	);
	await openForm();
	await page.getByLabelText('Schema URL', { exact: true }).fill(sourceURL);
	await page.getByRole('button', { name: 'Import schema', exact: true }).click();
	await expect
		.element(page.getByRole('alert'))
		.toHaveTextContent(
			'400 /mcp-catalogs/test/openapi/import: schema must be a JSON or YAML document'
		);
	await expect.element(page.getByRole('button', { name: 'Save', exact: true })).toBeVisible();
	mockImport();
	await page.getByRole('button', { name: 'Import schema', exact: true }).click();
	await expect
		.element(page.getByRole('status'))
		.toHaveTextContent('Schema imported. The snapshot will be saved with this entry.');
});

it('preserves an existing snapshot without importing', async () => {
	const imported = mockImport();
	const entry = {
		...createMCPCatalogEntryResponse,
		manifest: {
			...createMCPCatalogEntryResponse.manifest,
			runtime: 'openapi',
			openAPIConfig: {
				source: { url: sourceURL },
				schema
			}
		}
	} as MCPCatalogEntry;
	await render(CatalogServerForm, { id: 'test', entity: 'catalog', entry });
	await expect
		.element(page.getByRole('status'))
		.toHaveTextContent('Schema imported. The snapshot will be saved with this entry.');
	expect(imported).not.toHaveBeenCalled();
	expect(entry.manifest.openAPIConfig?.schema).toEqual(schema);
});

it.each([
	{ name: 'api.json', content: JSON.stringify(schema) },
	{
		name: 'api.yaml',
		content: 'openapi: 3.1.0\ninfo:\n  title: Example\n  version: "1"\npaths: {}'
	}
])(
	'uploads $name as source content and supports an API without credentials',
	async ({ name, content }) => {
		const imported = vi.fn();
		worker.use(
			http.post('/api/mcp-catalogs/test/openapi/import', async ({ request }) => {
				imported(await request.json());
				return HttpResponse.json({
					schema,
					baseURL: 'https://example.com',
					suggestedHeaders: [],
					suggestedMetadata: { name: 'Example' }
				});
			})
		);
		await openForm();
		await page.getByLabelText('Schema source').selectOptions('file');
		await page.getByLabelText('Schema file', { exact: true }).upload(new File([content], name));
		await expect.element(page.getByText(name, { exact: true })).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Replace file', exact: true }))
			.toBeVisible();
		await page.getByRole('button', { name: 'Import schema', exact: true }).click();
		await expect
			.element(page.getByRole('status'))
			.toHaveTextContent('Schema imported. The snapshot will be saved with this entry.');
		expect(imported).toHaveBeenCalledWith({ source: { content } });
		await expect.element(page.getByLabelText('Value Prefix')).not.toBeInTheDocument();
	}
);

it('shows an existing uploaded schema without asking users to select it again', async () => {
	await render(CatalogServerForm, {
		id: 'test',
		entity: 'catalog',
		entry: {
			...createMCPCatalogEntryResponse,
			manifest: {
				...createMCPCatalogEntryResponse.manifest,
				runtime: 'openapi',
				openAPIConfig: { source: { content: JSON.stringify(schema) }, schema }
			}
		} as MCPCatalogEntry
	});
	await expect.element(page.getByText('Uploaded schema', { exact: true })).toBeVisible();
	await expect
		.element(page.getByRole('status'))
		.toHaveTextContent('Schema imported. The snapshot will be saved with this entry.');
	await page.getByRole('button', { name: 'Replace schema', exact: true }).click();
	await expect.element(page.getByText('Saved schema', { exact: true })).toBeVisible();
	await expect
		.element(page.getByText('The existing schema is kept unless you choose a replacement.'))
		.toBeVisible();
	await expect
		.element(page.getByRole('button', { name: 'Replace file', exact: true }))
		.toBeEnabled();
});

it('opens the file picker only from the choose button, not the heading', async () => {
	await openForm();
	await page.getByLabelText('Schema source').selectOptions('file');
	const pickerClick = vi.fn();
	page
		.getByLabelText('Schema file', { exact: true })
		.element()
		.addEventListener('click', (event) => {
			// Observe activation without opening an operating-system dialog in the test.
			event.preventDefault();
			pickerClick();
		});
	await page.getByText('Schema file', { exact: true }).click();
	expect(pickerClick).not.toHaveBeenCalled();
	await page.getByRole('button', { name: 'Choose file', exact: true }).click();
	expect(pickerClick).toHaveBeenCalledOnce();
});

it('rejects oversized files before import', async () => {
	const imported = mockImport();
	await openForm();
	await page.getByLabelText('Schema source').selectOptions('file');
	await page
		.getByLabelText('Schema file', { exact: true })
		.upload(new File([new Uint8Array(1024 * 1024 + 1)], 'large.json'));
	await expect
		.element(page.getByRole('alert'))
		.toHaveTextContent('Schema files must be no larger than 1 MiB.');
	await expect
		.element(page.getByRole('button', { name: 'Import schema', exact: true }))
		.toBeDisabled();
	expect(imported).not.toHaveBeenCalled();
});

it('shows stored configuration read-only without offering import or source edits', async () => {
	await render(CatalogServerForm, {
		id: 'test',
		entity: 'catalog',
		readonly: true,
		entry: {
			...createMCPCatalogEntryResponse,
			manifest: {
				...createMCPCatalogEntryResponse.manifest,
				runtime: 'openapi',
				openAPIConfig: {
					source: { url: sourceURL },
					schema
				}
			}
		} as MCPCatalogEntry
	});
	await expect.element(page.getByText(sourceURL, { exact: true })).toBeVisible();
	await expect
		.element(page.getByRole('button', { name: 'Replace schema', exact: true }))
		.not.toBeInTheDocument();
	await expect
		.element(page.getByLabelText('API request base URL', { exact: false }))
		.toBeDisabled();
	await expect
		.element(page.getByRole('button', { name: 'Import schema', exact: true }))
		.not.toBeInTheDocument();
});

it('keeps edited header flags and prefixes and avoids duplicates on another import', async () => {
	const imported = mockImport();
	await openForm();
	await page.getByLabelText('Schema URL', { exact: true }).fill(sourceURL);
	await page.getByRole('button', { name: 'Import schema', exact: true }).click();
	await expect
		.element(page.getByRole('status'))
		.toHaveTextContent('Schema imported. The snapshot will be saved with this entry.');
	await page.getByLabelText('Value Prefix').fill('Token ');
	const required = page.getByRole('switch', { name: 'Required', exact: true });
	const sensitive = page.getByRole('switch', { name: 'Sensitive', exact: true });
	await expect.element(required).toBeChecked();
	await expect.element(sensitive).toBeChecked();
	await required.click();
	await sensitive.click();
	await page.getByRole('button', { name: 'Replace schema', exact: true }).click();
	await page.getByRole('button', { name: 'Import schema', exact: true }).click();
	await vi.waitFor(() => expect(imported).toHaveBeenCalledTimes(2));
	await page.getByRole('button', { name: 'Keep current values', exact: true }).click();
	await expect
		.element(page.getByRole('status'))
		.toHaveTextContent('Schema imported. The snapshot will be saved with this entry.');
	await expect.element(page.getByLabelText('Value Prefix')).toHaveValue('Token ');
	await expect.element(required).not.toBeChecked();
	await expect.element(sensitive).not.toBeChecked();
});

it('locks source edits during import and ignores the result after leaving the form', async () => {
	let finishImport!: () => void;
	const responseReady = new Promise<void>((resolve) => {
		finishImport = resolve;
	});
	worker.use(
		http.post('/api/mcp-catalogs/test/openapi/import', async () => {
			await responseReady;
			return HttpResponse.json({
				schema,
				baseURL: 'https://example.com',
				suggestedHeaders: [suggestedHeader],
				suggestedMetadata: { name: 'Example' }
			});
		})
	);
	const form = await openForm();
	await page.getByLabelText('Schema URL', { exact: true }).fill(sourceURL);
	await page.getByRole('button', { name: 'Import schema', exact: true }).click();
	await expect.element(page.getByLabelText('Schema URL', { exact: true })).toBeDisabled();
	await expect.element(page.getByRole('button', { name: 'Save', exact: true })).toBeVisible();
	await form.unmount();
	finishImport();
	await expect.element(page.getByRole('status')).not.toBeInTheDocument();
	await expect.element(page.getByLabelText('Value Prefix')).not.toBeInTheDocument();
});

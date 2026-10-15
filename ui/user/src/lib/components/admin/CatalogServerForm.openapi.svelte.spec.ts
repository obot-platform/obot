import { CATALOG_SERVER_FIELD_IDS } from '$lib/constants';
import type { MCPCatalogEntry } from '$lib/services';
import { darkMode, version } from '$lib/stores';
import { createMCPCatalogEntryResponse, getVersionResponse } from '../../../tests/mocks/data';
import { worker } from '../../../tests/mocks/worker';
import CatalogServerForm from './CatalogServerForm.svelte';
import { http, HttpResponse } from 'msw';
import { tick } from 'svelte';
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

async function selectOpenAPISourceFile() {
	await page.getByRole('combobox', { name: 'Schema source' }).click();
	await page.getByRole('button', { name: 'Upload file', exact: true }).click();
}

function mockImport() {
	const request = vi.fn();
	worker.use(
		http.post('/api/openapi-import', async ({ request: req }) => {
			request(await req.json());
			return HttpResponse.json({
				baseURL: 'https://example.com/api',
				destinationValidation: { valid: true },
				suggestedHeaders: [suggestedHeader],
				suggestedMetadata: { name: 'Example' }
			});
		})
	);
	return request;
}

it('trims import and destination URLs, then saves only the source and header prefix', async () => {
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
	await page.getByLabelText('Schema URL', { exact: true }).fill(`  ${sourceURL}  `);
	await page.getByRole('button', { name: 'Import schema', exact: true }).click();
	await expect
		.element(page.getByRole('status'))
		.toHaveTextContent('Schema imported. The snapshot will be saved with this entry.');
	expect(imported).toHaveBeenCalledWith({ source: { url: sourceURL } });
	await expect.element(page.getByLabelText('Value Prefix')).toHaveValue('Bearer ');
	await page
		.getByLabelText('API request base URL', { exact: false })
		.fill('  https://override.example.com  ');
	await page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.name}`).fill('Example API');
	await page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.shortDescription}`).fill('An example API');
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await vi.waitFor(() => expect(saved).toHaveBeenCalled());
	expect(saved.mock.calls[0][0]).toMatchObject({
		runtime: 'openapi',
		openAPIConfig: {
			source: { url: sourceURL },
			baseURL: 'https://override.example.com'
		},
		config: [suggestedHeader]
	});
	expect(saved.mock.calls[0][0].openAPIConfig).not.toHaveProperty('schema');
});

it('rejects OpenAPI header names that differ only by case', async () => {
	mockImport();
	const saved = vi.fn();
	worker.use(
		http.post('/api/mcp-catalogs/test/entries', async () => {
			saved();
			return HttpResponse.json(createMCPCatalogEntryResponse);
		})
	);
	await openForm();
	await page.getByLabelText('Schema URL', { exact: true }).fill(sourceURL);
	await page.getByRole('button', { name: 'Import schema', exact: true }).click();
	await expect.element(page.getByLabelText('Value Prefix')).toHaveValue('Bearer ');
	await page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.addConfigurationBtn}`).click();
	await page.getByCSS(`#env-key-${CATALOG_SERVER_FIELD_IDS.env}-1`).fill('authorization');
	await page.getByCSS(`#env-name-${CATALOG_SERVER_FIELD_IDS.env}-1`).fill('Second auth header');
	await page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.shortDescription}`).fill('An example API');
	await page.getByRole('button', { name: 'Save', exact: true }).click();

	await expect
		.element(page.getByRole('alert'))
		.toHaveTextContent(
			'OpenAPI credential header names must be unique, regardless of letter case.'
		);
	expect(saved).not.toHaveBeenCalled();
});

it('keeps imported headers required by default and allows saving optional headers', async () => {
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
	const required = page.getByRole('switch', { name: 'Required', exact: true });
	await expect.element(required).toBeEnabled();
	await expect.element(required).toBeChecked();
	await required.click();
	await page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.shortDescription}`).fill('An example API');
	await page.getByRole('button', { name: 'Save', exact: true }).click();

	await vi.waitFor(() => expect(saved).toHaveBeenCalledOnce());
	expect(saved.mock.calls[0][0].config).toEqual([{ ...suggestedHeader, required: false }]);
});

it('allows removing schema-suggested credential headers before saving', async () => {
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
	await expect.element(page.getByRole('switch', { name: 'Required', exact: true })).toBeChecked();
	await page.getByRole('button', { name: 'Remove configuration', exact: true }).click();
	await expect
		.element(page.getByRole('switch', { name: 'Required', exact: true }))
		.not.toBeInTheDocument();
	await page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.shortDescription}`).fill('An example API');
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await vi.waitFor(() => expect(saved).toHaveBeenCalledOnce());
	expect(saved.mock.calls[0][0].config).toEqual([]);
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
	worker.use(
		http.post('/api/openapi-import', () =>
			HttpResponse.json({
				suggestedMetadata: {
					name: 'Context API',
					shortDescription: 'Find documentation',
					description: 'Full API description',
					icon: 'https://example.com/logo.svg'
				},
				baseURL: 'https://example.com/api',
				destinationValidation: { valid: true },
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
		openAPIConfig: { source: { url: sourceURL } }
	});
	expect(saved.mock.calls[0][0].openAPIConfig).not.toHaveProperty('schema');
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

it('preserves existing metadata after importing a replacement', async () => {
	worker.use(
		http.post('/api/openapi-import', () =>
			HttpResponse.json({
				suggestedMetadata: {
					name: 'New name',
					shortDescription: 'New summary',
					description: 'New description',
					icon: 'https://example.com/new.png'
				},
				baseURL: 'https://example.com/api',
				destinationValidation: { valid: true },
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
			icon: 'https://example.com/existing.png',
			runtime: 'openapi',
			openAPIConfig: { source: { url: sourceURL }, baseURL: 'https://override.example.com' }
		}
	} as MCPCatalogEntry;
	await render(CatalogServerForm, { id: 'test', entity: 'catalog', entry });
	await page.getByRole('button', { name: 'Replace schema', exact: true }).click();
	await page.getByRole('button', { name: 'Import schema', exact: true }).click();
	await expect
		.element(page.getByRole('status'))
		.toHaveTextContent('Schema imported. The snapshot will be saved with this entry.');
	expect(document.querySelector('dialog[open]')).toBeNull();
	await expect.element(page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.name}`)).toHaveValue('My name');
	await expect
		.element(page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.shortDescription}`))
		.toHaveValue('My summary');
	await expect.element(page.getByText('My description', { exact: true })).toBeVisible();
	await expect
		.element(page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.icon}`))
		.toHaveValue('https://example.com/existing.png');
	await expect
		.element(page.getByLabelText('API request base URL', { exact: false }))
		.toHaveValue('https://override.example.com');
	expect(entry.manifest.openAPIConfig).not.toHaveProperty('schema');
});

it('requires a base URL inline when the import has no usable server URL', async () => {
	const saved = vi.fn();
	worker.use(
		http.post('/api/openapi-import', async ({ request }) => {
			const body = (await request.json()) as { baseURL?: string };
			return HttpResponse.json({
				baseURL: body.baseURL ?? '',
				destinationValidation: body.baseURL
					? { valid: true }
					: { valid: false, error: 'no usable server URL; configure baseURL' },
				suggestedHeaders: [],
				suggestedMetadata: { name: 'Example', shortDescription: 'Example API' }
			});
		}),
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
		.element(page.getByText('no usable server URL; configure baseURL', { exact: false }))
		.toBeVisible();
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await expect.element(baseURL).toHaveFocus();
	await expect.element(baseURL).toHaveAttribute('aria-invalid', 'true');
	await expect.element(baseURL).toHaveClass('error');
	expect(saved).not.toHaveBeenCalled();
	await baseURL.fill('https://petstore3.swagger.io/api/v3');
	await expect.element(baseURL).not.toHaveAttribute('aria-invalid', 'true');
	await expect
		.element(page.getByText('no usable server URL; configure baseURL', { exact: false }))
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
		http.post('/api/openapi-import', () =>
			HttpResponse.json({
				baseURL: 'https://example.com/api',
				destinationValidation: { valid: true },
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
	const originalDarkMode = darkMode.isDark;
	try {
		darkMode.setDark(!originalDarkMode);
		await tick();
		await expect
			.element(page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.description}-container .cm-content`))
			.toHaveTextContent('Imported API description');
	} finally {
		darkMode.setDark(originalDarkMode);
	}
	await page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.shortDescription}`).fill('API summary');
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await vi.waitFor(() =>
		expect(saved).toHaveBeenCalledWith(
			expect.objectContaining({ name: 'Example', description: 'Imported API description' })
		)
	);
});

it('uses the shared import endpoint in a workspace and stages source changes', async () => {
	const imported = mockImport();
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
			'/api/openapi-import',
			() => new HttpResponse('schema must be a JSON or YAML document', { status: 400 })
		)
	);
	await openForm();
	await page.getByLabelText('Schema URL', { exact: true }).fill(sourceURL);
	await page.getByRole('button', { name: 'Import schema', exact: true }).click();
	await expect
		.element(page.getByRole('alert'))
		.toHaveTextContent('400 /openapi-import: schema must be a JSON or YAML document');
	await expect.element(page.getByRole('button', { name: 'Save', exact: true })).toBeVisible();
	mockImport();
	await page.getByRole('button', { name: 'Import schema', exact: true }).click();
	await expect
		.element(page.getByRole('status'))
		.toHaveTextContent('Schema imported. The snapshot will be saved with this entry.');
});

it('loads an existing entry without a snapshot and submits only its URL on update', async () => {
	const imported = mockImport();
	const entry = {
		...createMCPCatalogEntryResponse,
		manifest: {
			...createMCPCatalogEntryResponse.manifest,
			runtime: 'openapi',
			config: [],
			openAPIConfig: {
				source: { url: sourceURL }
			}
		}
	} as MCPCatalogEntry;
	const saved = vi.fn();
	worker.use(
		http.put(`/api/mcp-catalogs/test/entries/${entry.id}`, async ({ request }) => {
			saved(await request.json());
			return HttpResponse.json(entry);
		})
	);
	await render(CatalogServerForm, { id: 'test', entity: 'catalog', entry });
	await expect
		.element(page.getByRole('status'))
		.toHaveTextContent('Schema imported. The snapshot will be saved with this entry.');
	expect(imported).not.toHaveBeenCalled();
	expect(entry.manifest.openAPIConfig).not.toHaveProperty('schema');
	await page.getByRole('button', { name: 'Update', exact: true }).click();
	await vi.waitFor(() => expect(saved).toHaveBeenCalled());
	expect(saved.mock.calls[0][0].openAPIConfig).toMatchObject({ source: { url: sourceURL } });
	expect(saved.mock.calls[0][0].openAPIConfig).not.toHaveProperty('schema');
});

it('reopens the saved schema destination as an editable value without importing again', async () => {
	const imported = mockImport();
	const baseURL = 'https://schema-api.example.com/v2/';
	const entry = {
		...createMCPCatalogEntryResponse,
		manifest: {
			...createMCPCatalogEntryResponse.manifest,
			runtime: 'openapi',
			config: [],
			openAPIConfig: {
				source: { url: sourceURL },
				baseURL
			}
		}
	} as MCPCatalogEntry;
	await render(CatalogServerForm, { id: 'test', entity: 'catalog', entry });
	const input = page.getByLabelText('API request base URL', { exact: false });
	await expect.element(input).toHaveValue(baseURL);
	await expect.element(input).toBeEnabled();
	expect(imported).not.toHaveBeenCalled();
	await input.fill('');
	await expect.element(input).toHaveValue('');
	await expect.element(input).not.toBeRequired();
});

it('does not save the old schema when replacement starts during destination validation', async () => {
	let finishValidation!: () => void;
	const validationReady = new Promise<void>((resolve) => {
		finishValidation = resolve;
	});
	const validating = vi.fn();
	const saved = vi.fn();
	worker.use(
		http.post('/api/openapi-import', async ({ request }) => {
			validating(await request.json());
			await validationReady;
			return HttpResponse.json({ destinationValidation: { valid: true } });
		}),
		http.post('/api/mcp-catalogs/test/entries', async () => {
			saved();
			return HttpResponse.json(createMCPCatalogEntryResponse);
		})
	);
	await render(CatalogServerForm, {
		id: 'test',
		entity: 'catalog',
		type: 'openapi',
		initialOpenAPIImport: {
			config: { source: { url: sourceURL } },
			result: {
				baseURL: 'https://example.com/api',
				destinationValidation: { valid: true },
				suggestedHeaders: [],
				suggestedMetadata: { name: 'Example', shortDescription: 'Example API' }
			}
		}
	});
	await page
		.getByLabelText('API request base URL', { exact: false })
		.fill('https://override.example.com');
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await vi.waitFor(() => expect(validating).toHaveBeenCalledOnce());
	await page.getByRole('button', { name: 'Replace schema', exact: true }).click();
	await page.getByLabelText('Schema URL', { exact: true }).fill('https://example.com/new.json');
	finishValidation();
	await expect
		.element(page.getByRole('button', { name: 'Save', exact: true }))
		.toHaveAttribute('aria-busy', 'false');
	expect(saved).not.toHaveBeenCalled();
	await expect
		.element(page.getByLabelText('Schema URL', { exact: true }))
		.toHaveValue('https://example.com/new.json');
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
		const saved = vi.fn();
		worker.use(
			http.post('/api/openapi-import', async ({ request }) => {
				imported(await request.json());
				return HttpResponse.json({
					baseURL: 'https://example.com',
					destinationValidation: { valid: true },
					suggestedHeaders: [],
					suggestedMetadata: { name: 'Example' }
				});
			}),
			http.post('/api/mcp-catalogs/test/entries', async ({ request }) => {
				saved(await request.json());
				return HttpResponse.json(createMCPCatalogEntryResponse);
			})
		);
		await openForm();
		await selectOpenAPISourceFile();
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
		await page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.shortDescription}`).fill('Uploaded API');
		await page.getByRole('button', { name: 'Save', exact: true }).click();
		await vi.waitFor(() => expect(saved).toHaveBeenCalled());
		expect(saved.mock.calls[0][0].openAPIConfig.source).toEqual({ content });
		expect(saved.mock.calls[0][0].openAPIConfig).not.toHaveProperty('schema');
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
				openAPIConfig: { source: { content: JSON.stringify(schema) } }
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
	await selectOpenAPISourceFile();
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

it('re-reads a file with the same name after Replace file', async () => {
	const imported = vi.fn();
	worker.use(
		http.post('/api/openapi-import', async ({ request }) => {
			imported(await request.json());
			return HttpResponse.json({
				baseURL: 'https://example.com/api',
				destinationValidation: { valid: true },
				suggestedHeaders: [],
				suggestedMetadata: { name: 'Example' }
			});
		})
	);
	await openForm();
	await selectOpenAPISourceFile();
	const input = page.getByLabelText('Schema file', { exact: true }).element() as HTMLInputElement;
	await page.getByLabelText('Schema file', { exact: true }).upload(new File(['first'], 'api.json'));
	await expect.element(page.getByRole('button', { name: 'Replace file' })).toBeVisible();
	expect(input.value).not.toBe('');
	input.addEventListener('click', (event) => event.preventDefault());
	await page.getByRole('button', { name: 'Replace file' }).click();
	expect(input.value).toBe('');
	await page
		.getByLabelText('Schema file', { exact: true })
		.upload(new File(['updated'], 'api.json'));
	await page.getByRole('button', { name: 'Import schema' }).click();
	await vi.waitFor(() => expect(imported).toHaveBeenCalledWith({ source: { content: 'updated' } }));
});

it('rejects oversized files before import', async () => {
	const imported = mockImport();
	await openForm();
	await selectOpenAPISourceFile();
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
					source: { url: sourceURL }
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
	await expect
		.element(page.getByRole('status'))
		.toHaveTextContent('Schema imported. The snapshot will be saved with this entry.');
	expect(document.querySelector('dialog[open]')).toBeNull();
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
		http.post('/api/openapi-import', async () => {
			await responseReady;
			return HttpResponse.json({
				baseURL: 'https://example.com',
				destinationValidation: { valid: true },
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

it.each([
	{ serverURL: 'http://api.example.com', error: 'API destination must use HTTPS', initial: false },
	{ serverURL: 'https://127.0.0.1', error: 'API destination is blocked', initial: false },
	{ serverURL: 'http://api.example.com', error: 'API destination must use HTTPS', initial: true },
	{ serverURL: 'https://127.0.0.1', error: 'API destination is blocked', initial: true }
])(
	'requires an override for $serverURL after import (initial draft: $initial)',
	async ({ serverURL, error, initial }) => {
		const result = {
			baseURL: serverURL,
			destinationValidation: { valid: false, error },
			suggestedHeaders: [
				{ ...suggestedHeader, usage: 'header' as const, value: '', description: '' }
			],
			suggestedMetadata: { name: 'Example', shortDescription: 'Example API' }
		};
		const saved = vi.fn();
		const imported = vi.fn();
		worker.use(
			http.post('/api/openapi-import', async ({ request }) => {
				const body = await request.json();
				imported(body);
				return HttpResponse.json(
					body && typeof body === 'object' && 'baseURL' in body
						? { ...result, destinationValidation: { valid: true } }
						: result
				);
			}),
			http.post('/api/mcp-catalogs/test/entries', async ({ request }) => {
				saved(await request.json());
				return HttpResponse.json(createMCPCatalogEntryResponse);
			})
		);
		await render(CatalogServerForm, {
			id: 'test',
			entity: 'catalog',
			type: 'openapi',
			initialOpenAPIImport: initial ? { config: { source: { url: sourceURL } }, result } : undefined
		});
		if (!initial) {
			await expect
				.element(page.getByLabelText('API request base URL', { exact: false }))
				.not.toBeInTheDocument();
			await page.getByLabelText('Schema URL', { exact: true }).fill(sourceURL);
			await page.getByRole('button', { name: 'Import schema', exact: true }).click();
			await vi.waitFor(() => expect(imported).toHaveBeenCalledWith({ source: { url: sourceURL } }));
		}
		const baseURL = page.getByLabelText('API request base URL', { exact: false });
		await expect.element(baseURL).toHaveValue('');
		await expect.element(baseURL).toHaveAttribute('placeholder', 'https://api.example.com/v1');
		await expect.element(baseURL).toBeRequired();
		await expect.element(page.getByText(error, { exact: false })).toBeVisible();
		await expect.element(page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.name}`)).toHaveValue('Example');
		await expect.element(page.getByLabelText('Value Prefix')).toHaveValue('Bearer ');
		await page.getByRole('button', { name: 'Save', exact: true }).click();
		expect(saved).not.toHaveBeenCalled();
		await expect.element(baseURL).toHaveAttribute('aria-invalid', 'true');
		await baseURL.fill('https://override.example.com');
		await expect.element(page.getByText(error, { exact: false })).not.toBeInTheDocument();
		await page.getByRole('button', { name: 'Save', exact: true }).click();
		await vi.waitFor(() => expect(saved).toHaveBeenCalledOnce());
		expect(saved.mock.calls[0][0].openAPIConfig).toEqual({
			source: { url: sourceURL },
			baseURL: 'https://override.example.com',
			egressDomains: []
		});
	}
);

it.each([
	{ baseURL: 'foo', error: 'baseURL must be absolute HTTP(S)' },
	{ baseURL: 'https://127.0.0.1', error: 'API destination is blocked' },
	{ baseURL: 'http://api.example.com', error: 'API destination must use HTTPS' },
	{ baseURL: 'https://user:pass@example.com', error: 'baseURL must be absolute HTTP(S)' }
])('blocks saving an invalid override $baseURL', async ({ baseURL, error }) => {
	const imported = vi.fn();
	const saved = vi.fn();
	worker.use(
		http.post('/api/openapi-import', async ({ request }) => {
			const body = (await request.json()) as { baseURL?: string };
			imported(body);
			return HttpResponse.json({
				baseURL: body.baseURL ?? 'https://example.com/api',
				destinationValidation: body.baseURL ? { valid: false, error } : { valid: true },
				suggestedHeaders: [],
				suggestedMetadata: { name: 'Example', shortDescription: 'Example API' }
			});
		}),
		http.post('/api/mcp-catalogs/test/entries', async ({ request }) => {
			saved(await request.json());
			return HttpResponse.json(createMCPCatalogEntryResponse);
		})
	);
	await openForm();
	await page.getByLabelText('Schema URL', { exact: true }).fill(sourceURL);
	await page.getByRole('button', { name: 'Import schema', exact: true }).click();
	const override = page.getByLabelText('API request base URL', { exact: false });
	await override.fill(baseURL);
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await vi.waitFor(() =>
		expect(imported).toHaveBeenCalledWith({ source: { url: sourceURL }, baseURL })
	);
	expect(saved).not.toHaveBeenCalled();
	await expect.element(override).toHaveAttribute('aria-invalid', 'true');
	await expect.element(page.getByText(error, { exact: false })).toBeVisible();
});

it('edits and saves an existing entry without receiving a schema snapshot', async () => {
	const entry = {
		...createMCPCatalogEntryResponse,
		manifest: {
			...createMCPCatalogEntryResponse.manifest,
			runtime: 'openapi',
			config: [],
			openAPIConfig: { source: { url: sourceURL } }
		}
	} as MCPCatalogEntry;
	const saved = vi.fn();
	const imported = vi.fn();
	worker.use(
		http.post('/api/openapi-import', () => {
			imported();
			return HttpResponse.json({});
		}),
		http.put(`/api/mcp-catalogs/test/entries/${entry.id}`, async ({ request }) => {
			saved(await request.json());
			return HttpResponse.json(entry);
		})
	);
	await render(CatalogServerForm, { id: 'test', entity: 'catalog', entry });
	await expect
		.element(page.getByRole('status'))
		.toHaveTextContent('Schema imported. The snapshot will be saved with this entry.');
	await expect.element(page.getByLabelText('Schema URL', { exact: true })).not.toBeInTheDocument();
	await expect
		.element(page.getByLabelText('API request base URL', { exact: false }))
		.not.toBeRequired();
	await page.getByRole('button', { name: 'Update', exact: true }).click();
	await vi.waitFor(() => expect(saved).toHaveBeenCalledOnce());
	expect(imported).not.toHaveBeenCalled();
	expect(saved.mock.calls[0][0].openAPIConfig).toMatchObject({ source: { url: sourceURL } });
	expect(saved.mock.calls[0][0].openAPIConfig).not.toHaveProperty('schema');
});

it.each(['inline', 'initial draft', 'replacement'])(
	'completes %s import with null header suggestions and saves an API without credentials',
	async (mode) => {
		const result = {
			baseURL: 'https://example.com/api',
			destinationValidation: { valid: true },
			suggestedHeaders: null,
			suggestedMetadata: { name: 'Public API', shortDescription: 'Public API description' }
		};
		const saved = vi.fn();
		const entry = {
			...createMCPCatalogEntryResponse,
			manifest: {
				...createMCPCatalogEntryResponse.manifest,
				runtime: 'openapi',
				config: [],
				openAPIConfig: { source: { url: sourceURL } }
			}
		} as MCPCatalogEntry;
		worker.use(
			http.post('/api/openapi-import', () => HttpResponse.json(result)),
			http.post('/api/mcp-catalogs/test/entries', async ({ request }) => {
				saved(await request.json());
				return HttpResponse.json(createMCPCatalogEntryResponse);
			}),
			http.put(`/api/mcp-catalogs/test/entries/${entry.id}`, async ({ request }) => {
				saved(await request.json());
				return HttpResponse.json(entry);
			})
		);
		// The initial dialog receives the normalized service result before opening this form.
		const { importOpenAPI } = await import('$lib/services/openapi');
		const initialOpenAPIImport =
			mode === 'initial draft'
				? {
						config: { source: { url: sourceURL } },
						result: await importOpenAPI({ source: { url: sourceURL } })
					}
				: undefined;
		await render(CatalogServerForm, {
			id: 'test',
			entity: 'catalog',
			type: 'openapi',
			entry: mode === 'replacement' ? entry : undefined,
			initialOpenAPIImport
		});
		if (mode !== 'initial draft') {
			if (mode === 'replacement')
				await page.getByRole('button', { name: 'Replace schema', exact: true }).click();
			await page.getByLabelText('Schema URL', { exact: true }).fill(sourceURL);
			await page.getByRole('button', { name: 'Import schema', exact: true }).click();
		}
		await expect
			.element(page.getByRole('status'))
			.toHaveTextContent('Schema imported. The snapshot will be saved with this entry.');
		await expect.element(page.getByLabelText('Value Prefix')).not.toBeInTheDocument();
		if (mode !== 'replacement')
			await expect
				.element(page.getByCSS(`#${CATALOG_SERVER_FIELD_IDS.name}`))
				.toHaveValue('Public API');
		await page
			.getByRole('button', { name: mode === 'replacement' ? 'Update' : 'Save', exact: true })
			.click();
		await vi.waitFor(() => expect(saved).toHaveBeenCalledOnce());
		expect(saved.mock.calls[0][0]).toMatchObject({
			config: [],
			openAPIConfig: { source: { url: sourceURL } }
		});
	}
);

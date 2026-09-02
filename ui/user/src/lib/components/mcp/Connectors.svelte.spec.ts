import { mcpServersAndEntries } from '$lib/stores';
import { openUrl } from '$lib/utils';
import { createMCPCatalogEntry, createMCPCatalogServer } from '../../../tests/helpers/mcp';
import Connectors from './Connectors.svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page, userEvent } from 'vitest/browser';

const shortDescription =
	'A **formatted** description with [documentation](https://example.com/docs). ![Short demo](https://example.com/short-demo.gif)';
const fullDescription =
	'Full description that should not render. ![Large demo](https://example.com/large-demo.gif)';

vi.mock('$lib/utils', async (importOriginal) => ({
	...(await importOriginal<typeof import('$lib/utils')>()),
	openUrl: vi.fn()
}));

describe('MCP Servers ConnectorsView', () => {
	beforeEach(() => {
		vi.mocked(openUrl).mockImplementation(() => undefined);
		const entry = createMCPCatalogEntry({
			id: 'deprecated-connected-entry',
			name: 'Deprecated Connected Server',
			manifest: {
				shortDescription,
				description: fullDescription,
				icon: 'https://example.com/icon.png',
				metadata: { deprecated: 'true' }
			}
		});
		const configuredServer = createMCPCatalogServer({
			id: 'configured-server',
			name: entry.manifest.name!,
			catalogEntryID: entry.id,
			userID: 'user-1'
		});

		mcpServersAndEntries.current = {
			entries: [entry],
			servers: [],
			userInstances: [],
			userConfiguredServers: [configuredServer],
			loading: false,
			lastFetched: null,
			isInitialized: true
		};
	});

	it('clamps the description independently from the title and status badges', async () => {
		render(Connectors, { query: 'Deprecated' });

		const descriptionPreview = page.getByText(/A formatted description with documentation/);
		await expect.element(page.getByText('Deprecated', { exact: true })).toBeVisible();
		await expect.element(page.getByText('Connected', { exact: true })).toBeVisible();
		await expect.element(descriptionPreview).toHaveClass(/line-clamp-2/);
		await expect.element(descriptionPreview).toHaveClass(/mt-1/);
		await expect.element(descriptionPreview).not.toHaveClass(/min-h-8/);
		await expect.element(descriptionPreview.locator('..')).not.toHaveClass(/line-clamp-2/);
		await expect.element(descriptionPreview.locator('strong')).toHaveTextContent('formatted');
		await expect
			.element(descriptionPreview.getByRole('link', { name: 'documentation' }))
			.toHaveAttribute('target', '_blank');
		await expect
			.element(page.getByText(/Full description that should not render/))
			.not.toBeInTheDocument();
		await expect.element(page.getByRole('img', { name: 'Short demo' })).not.toBeInTheDocument();
		await expect.element(page.getByRole('img', { name: 'Large demo' })).not.toBeInTheDocument();
	});

	it('defers loading connector icons', async () => {
		render(Connectors, { query: 'Deprecated' });

		const icon = page.getByRole('img', { name: 'Deprecated Connected Server' });
		await expect.element(icon).toHaveAttribute('loading', 'lazy');
		await expect.element(icon).toHaveAttribute('decoding', 'async');
	});

	it('keeps mouse and keyboard link activation from selecting the connector row', async () => {
		render(Connectors, { query: 'Deprecated' });

		const link = page.getByRole('link', { name: 'documentation' });
		const anchor = link.element() as HTMLAnchorElement;
		let linkActivations = 0;
		anchor.addEventListener('click', (event) => {
			event.preventDefault();
			linkActivations++;
		});

		await userEvent.click(anchor);
		expect(linkActivations).toBe(1);
		expect(openUrl).not.toHaveBeenCalled();

		anchor.focus();
		await userEvent.keyboard('{Enter}');
		expect(linkActivations).toBe(2);
		expect(openUrl).not.toHaveBeenCalled();
	});

	it('selects the connector when non-interactive row content is clicked', async () => {
		render(Connectors, { query: 'Deprecated' });

		await page.getByText('Deprecated Connected Server', { exact: true }).click();

		expect(openUrl).toHaveBeenCalledOnce();
	});

	it('shows the reauthenticate option for configured remote OAuth servers', async () => {
		const entry = createMCPCatalogEntry({
			id: 'remote-entry',
			name: 'Remote OAuth Server',
			runtime: 'remote',
			manifest: {
				remoteConfig: { fixedURL: 'https://example.com/mcp' }
			}
		});
		const server = createMCPCatalogServer({
			id: 'remote-server',
			name: entry.manifest.name!,
			runtime: 'remote',
			catalogEntryID: entry.id,
			userID: 'user-1',
			connectURL: 'https://obot.example.com/mcp/remote-server',
			oauthMetadata: {
				protectedResourceUrl: 'https://example.com/.well-known/oauth-protected-resource'
			}
		});
		mcpServersAndEntries.current = {
			entries: [entry],
			servers: [],
			userInstances: [],
			userConfiguredServers: [server],
			loading: false,
			lastFetched: null,
			isInitialized: true
		};

		render(Connectors);

		await page.getByRole('button', { name: 'Connect', exact: true }).click();
		await expect.element(page.getByRole('button', { name: 'Reauthenticate' })).toBeVisible();
	});

	it('marks detail navigation as coming from the connectors list', async () => {
		vi.mocked(openUrl).mockClear();
		render(Connectors, { query: 'Deprecated' });

		await page.getByRole('button', { name: /Deprecated Connected Server/ }).click();

		expect(openUrl).toHaveBeenCalledWith(
			'/mcp-servers/c/deprecated-connected-entry/instance/configured-server?from=connectors',
			false
		);
	});
});

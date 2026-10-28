import { createVMCP } from '../../../../../tests/helpers/mcp';
import { preparePageData } from '../../../../../tests/helpers/pageData';
import { getProfileResponse } from '../../../../../tests/mocks/data';
import { worker } from '../../../../../tests/mocks/worker';
import type { PageData } from './$types';
import SetupPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page, userEvent } from 'vitest/browser';

function mockSetupApis(status: { configured: boolean; configurationCheckHash?: string }) {
	const vmcp = createVMCP({ id: 'vmcp1setup', displayName: 'Setup vMCP' });
	const component = vmcp.components[0];
	component.configuration = [{ key: 'TOKEN', policy: 'userAllowed' }];
	component.catalogEntry.manifest.config = [
		{
			key: 'TOKEN',
			name: 'API token',
			description: '',
			value: '',
			required: true,
			sensitive: true,
			usage: 'env'
		}
	];
	const instance = {
		id: 'vmcpi1selected',
		vmcpID: vmcp.id,
		userID: getProfileResponse.id,
		created: '2026-01-01T00:00:00Z',
		status
	};
	const saved = vi.fn();
	const authentication = vi.fn();
	const created = vi.fn();
	const vmcpReads = vi.fn();
	worker.use(
		http.get('/api/vmcps/vmcp1setup', () => {
			vmcpReads();
			return HttpResponse.json(vmcp);
		}),
		http.get('/api/vmcp-instances/vmcpi1selected', () => HttpResponse.json(instance)),
		http.post('/api/vmcp-instances', () => {
			created();
			return HttpResponse.json(instance);
		}),
		http.post('/api/vmcp-instances/vmcpi1selected/reveal', () =>
			HttpResponse.json({ components: {} })
		),
		http.post('/api/vmcp-instances/vmcpi1selected/configure', async ({ request }) => {
			saved(await request.json());
			return HttpResponse.json(instance);
		}),
		http.get('/api/oauth/vmcp/:id', ({ params, request }) => {
			authentication(params.id, new URL(request.url).searchParams.get('oauth_auth_request'));
			return HttpResponse.json([
				{ mcpServerID: 'gmail', name: 'Gmail', authURL: `https://example.com/oauth/${params.id}` }
			]);
		})
	);
	return { authentication, component, created, instance, saved, vmcpReads };
}

async function renderSetupPage(compositeMcpId: string) {
	const data = await preparePageData<PageData>({
		compositeMcpId,
		oauthAuthRequestId: 'original-consent'
	});
	return { data, ...(await render(SetupPage, { data })) };
}

it('configures the selected instance before authenticating its components', async () => {
	const { authentication, component, created, instance, saved, vmcpReads } = mockSetupApis({
		configured: true,
		configurationCheckHash: 'checked'
	});

	await renderSetupPage(instance.id);
	await expect.element(page.getByCSS('input[name="API token"]')).toBeVisible();
	expect(authentication).not.toHaveBeenCalled();
	// The setup dialog loads the vMCP itself; the page does not fetch it a second time.
	expect(vmcpReads).toHaveBeenCalledOnce();
	await page.getByCSS('input[name="API token"]').fill('secret');
	await page.getByRole('button', { name: 'Update', exact: true }).click();
	await expect
		.element(page.getByRole('link', { name: 'Authenticate', exact: true }))
		.toHaveAttribute('href', 'https://example.com/oauth/vmcpi1selected');
	expect(saved).toHaveBeenCalledWith({ components: { [component.id!]: { TOKEN: 'secret' } } });
	expect(authentication).toHaveBeenCalledWith('vmcpi1selected', 'original-consent');
	expect(created).not.toHaveBeenCalled();
});

it('continues to authentication when setup is closed for a configured instance', async () => {
	const { authentication, instance, saved } = mockSetupApis({
		configured: true,
		configurationCheckHash: 'checked'
	});

	await renderSetupPage(instance.id);
	await expect.element(page.getByCSS('input[name="API token"]')).toBeVisible();
	await userEvent.keyboard('{Escape}');

	await expect
		.element(page.getByRole('link', { name: 'Authenticate', exact: true }))
		.toHaveAttribute('href', 'https://example.com/oauth/vmcpi1selected');
	expect(authentication).toHaveBeenCalledWith('vmcpi1selected', 'original-consent');
	expect(saved).not.toHaveBeenCalled();
});

it('explains that an unconfigured instance must finish setup before authenticating', async () => {
	const { authentication, instance } = mockSetupApis({ configured: false });

	await renderSetupPage(instance.id);
	await expect.element(page.getByCSS('input[name="API token"]')).toBeVisible();
	await userEvent.keyboard('{Escape}');

	await expect
		.element(page.getByRole('alert'))
		.toHaveTextContent('Finish configuring this vMCP before authenticating its servers.');
	await expect.element(page.getByRole('button', { name: 'Retry' })).toBeEnabled();
	expect(authentication).not.toHaveBeenCalled();

	await page.getByRole('button', { name: 'Retry' }).click();
	await expect.element(page.getByCSS('input[name="API token"]')).toBeVisible();
});

it('starts over when navigating to another connection', async () => {
	const { authentication, instance } = mockSetupApis({
		configured: true,
		configurationCheckHash: 'checked'
	});

	const { data, rerender } = await renderSetupPage(instance.id);
	await expect.element(page.getByCSS('input[name="API token"]')).toBeVisible();

	await rerender({ data: { ...data, compositeMcpId: 'vmcp1setup' } });
	await expect
		.element(page.getByRole('link', { name: 'Authenticate', exact: true }))
		.toHaveAttribute('href', 'https://example.com/oauth/vmcp1setup');
	await expect.element(page.getByCSS('input[name="API token"]')).not.toBeInTheDocument();
	expect(authentication).toHaveBeenCalledWith('vmcp1setup', 'original-consent');
	expect(authentication).not.toHaveBeenCalledWith('vmcpi1selected', expect.anything());
});

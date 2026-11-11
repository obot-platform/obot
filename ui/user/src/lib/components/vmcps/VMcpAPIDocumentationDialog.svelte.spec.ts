import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import { http, HttpResponse } from 'msw';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';
import VMcpAPIDocumentationDialog from './VMcpAPIDocumentationDialog.svelte';
import type { VMCP } from '$lib/services';

describe('VMcpAPIDocumentationDialog.svelte', () => {
	it('renders the read-only OpenAPI calling contract', async () => {
		await preparePageData({ profile: createMockProfile([]) });
		await worker.use(
			http.get('*/api/vmcps/vmcp1/api-proxy', () =>
				HttpResponse.json({
					vmcpID: 'vmcp1',
					components: [
						{
							id: 'component1',
							displayName: 'Business API',
							basePath: 'https://obot.example.test/api-proxy/vmcp1/component1',
							executionDigest: 'digest-1',
							operations: [
								{
									operationKey: 'GET /items/{id}',
									legacyToolName: 'get_item',
									displayName: 'Get item',
									description: 'Return one item',
									method: 'GET',
									pathTemplate: '/items/{id}',
									parameters: [
										{ name: 'id', in: 'path', required: true, schema: { type: 'string' } }
									],
									businessAuthRequired: true,
									example: 'GET https://obot.example.test/api-proxy/vmcp1/component1/items/<id>'
								}
							]
						}
					]
				})
			)
		);
		const vmcp = { id: 'vmcp1', displayName: 'Business vMCP', components: [] } as unknown as VMCP;
		const result = await render(VMcpAPIDocumentationDialog);
		await result.component.open(vmcp);

		await expect.element(page.getByText('HTTP API docs: Business vMCP')).toBeVisible();
		await expect
			.element(page.getByCSS('#base-component1'))
			.toHaveValue('https://obot.example.test/api-proxy/vmcp1/component1');
		await expect.element(page.getByText('Get item')).toBeVisible();
		await expect.element(page.getByText('/items/{id}')).toBeVisible();
		await expect.element(page.getByText('Business auth required')).toBeVisible();
		await expect.element(page.getByText('Return one item')).toBeVisible();
	});
});

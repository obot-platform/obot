import type { VMCPInstance } from '$lib/services';
import type { VMcpConnectOptions } from '$lib/services/vmcps/types';
import { profile, vmcpInstances } from '$lib/stores';
import * as url from '$lib/url';
import { preparePageData } from '../../../tests/helpers/pageData';
import VMcpCardActions from './VMcpCardActions.svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock(import('$lib/url'), { spy: true });

const VMCP_ID = 'vmcp1test';

function instanceForCurrentUser(): VMCPInstance {
	return {
		id: 'vmcpi1test',
		vmcpID: VMCP_ID,
		userID: profile.current.id,
		created: '2026-01-01T00:00:00Z'
	};
}

async function renderActions({ withInstance = false } = {}) {
	const onConnect = vi.fn<(options?: VMcpConnectOptions) => void>();
	await preparePageData();
	vmcpInstances.current = {
		items: withInstance ? [instanceForCurrentUser()] : [],
		loading: false
	};
	render(VMcpCardActions, {
		id: VMCP_ID,
		connectURL: 'https://example.com/connect',
		onConnect
	});
	return { onConnect };
}

describe('VMcpCardActions.svelte', () => {
	beforeEach(() => {
		vmcpInstances.current = { items: [], loading: false };
		vi.mocked(url.goto).mockImplementation(() => undefined as never);
	});

	afterEach(() => {
		vmcpInstances.current = { items: [], loading: false };
		vi.mocked(url.goto).mockReset();
	});

	it('opens Connect when the user has no instance, then navigates after connect', async () => {
		const { onConnect } = await renderActions();

		await page.getByRole('button', { name: 'Test vMCP' }).click();

		expect(onConnect).toHaveBeenCalledTimes(1);
		expect(url.goto).not.toHaveBeenCalled();

		onConnect.mock.calls[0][0]?.onConnected?.();
		expect(url.goto).toHaveBeenCalledWith(`/mcp-servers/test/${VMCP_ID}`);
	});

	it('navigates to the tester when the user already has an instance', async () => {
		const { onConnect } = await renderActions({ withInstance: true });

		await page.getByRole('button', { name: 'Test vMCP' }).click();

		expect(onConnect).not.toHaveBeenCalled();
		expect(url.goto).toHaveBeenCalledWith(`/mcp-servers/test/${VMCP_ID}`);
	});
});

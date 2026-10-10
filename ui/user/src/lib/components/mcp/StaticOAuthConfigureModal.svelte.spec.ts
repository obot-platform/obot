import { page as appPage } from '$app/state';
import { goto } from '$lib/url';
import StaticOAuthConfigureModal from './StaticOAuthConfigureModal.svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock('$lib/url', async (importOriginal) => ({
	...(await importOriginal<typeof import('$lib/url')>()),
	goto: vi.fn().mockResolvedValue(undefined)
}));

const savedCredentials = {
	clientID: 'client-id',
	clientSecret: 'client-secret'
};

const toolSetupPrompt = 'Would you like to return to the vMCP tool setup you were working on?';
const returnNote = 'You can stay on this server if you are not finished here.';

const returnPrompts = [
	{
		name: 'vMCP tool setup',
		redirect: '/vmcps/vmcp-1?modify-tools=component-1',
		destination: '/vmcps/vmcp-1?modify-tools=component-1',
		prompt: toolSetupPrompt
	},
	{
		name: 'vMCP inspector setup',
		redirect: '/vmcps/vmcp-1?inspector=vmcp-1',
		destination: '/vmcps/vmcp-1?inspector=vmcp-1',
		prompt: 'Would you like to return to the vMCP inspector?'
	},
	{
		name: 'vMCP connect',
		redirect: '/vmcps/vmcp-1?connect=vmcp-1',
		destination: '/vmcps/vmcp-1?connect=vmcp-1',
		prompt: 'Would you like to return to connecting to the vMCP?'
	},
	{
		name: 'the previous page',
		redirect: '/mcp-servers/c/salesforce',
		destination: '/mcp-servers/c/salesforce',
		prompt: 'Would you like to return to where you left off?'
	},
	{
		name: 'an absolute same-origin URL',
		redirect: () => new URL('/vmcps/vmcp-1?modify-tools=component-1#tools', testOrigin).href,
		destination: '/vmcps/vmcp-1?modify-tools=component-1#tools',
		prompt: toolSetupPrompt
	}
];

const workflowPath = '/vmcps/vmcp-1?modify-tools=component-1';
const serverPath = '/mcp-servers/c/salesforce';
const dropRedirectOptions = { replaceState: true, noScroll: true, keepFocus: true };

const crossOriginRedirects = [
	{
		name: 'another host',
		redirect: () => {
			const url = new URL(workflowPath, testOrigin);
			url.hostname = 'evil.example';
			return url.href;
		}
	},
	{
		name: 'another protocol',
		redirect: () => {
			const url = new URL(workflowPath, testOrigin);
			url.protocol = url.protocol === 'https:' ? 'http:' : 'https:';
			return url.href;
		}
	},
	{
		name: 'a protocol-relative URL',
		redirect: () => '//evil.example/vmcps/vmcp-1?modify-tools=component-1'
	}
];

function redirectValue(redirect: string | (() => string)) {
	return typeof redirect === 'function' ? redirect() : redirect;
}

// The browser harness page URL is not a same-origin http URL, so the modal would
// reject every oauth-redirect until the test installs one.
const testOrigin = 'http://localhost';
const originalUrl = Object.getOwnPropertyDescriptor(appPage, 'url');

function setOAuthRedirect(value?: string) {
	const url = new URL(serverPath, testOrigin);
	if (value !== undefined) {
		url.searchParams.set('oauth-redirect', value);
	}
	Object.defineProperty(appPage, 'url', {
		configurable: true,
		value: url
	});
}

function deferredSave() {
	let resolveSave: (() => void) | undefined;
	const onSave = vi.fn(
		() =>
			new Promise<void>((resolve) => {
				resolveSave = resolve;
			})
	);
	return {
		onSave,
		succeed() {
			resolveSave?.();
		}
	};
}

async function openModal(
	onSave: (credentials: { clientID: string; clientSecret: string }) => Promise<void>
) {
	const result = await render(StaticOAuthConfigureModal, { onSave });
	result.component.open();
	await expect.element(page.getByRole('heading', { name: 'Configure Static OAuth' })).toBeVisible();
	return result;
}

async function fillAndSave() {
	await page.getByLabelText('Client ID', { exact: true }).fill('  client-id  ');
	await page.getByLabelText('Client Secret (optional)', { exact: true }).fill('  client-secret  ');
	await page.getByRole('button', { name: 'Save', exact: true }).click();
}

function expectOAuthRedirectDropped() {
	const [url, options] = vi.mocked(goto).mock.calls[0] ?? [];
	expect(url).toBeInstanceOf(URL);
	expect((url as URL).href).toBe(new URL(serverPath, testOrigin).href);
	expect(options).toEqual(dropRedirectOptions);
}

async function expectReturnPrompt(prompt: string) {
	await expect.element(page.getByText('Go Back?', { exact: true })).toBeVisible();
	await expect.element(page.getByText(prompt, { exact: true })).toBeVisible();
	await expect.element(page.getByText(returnNote, { exact: true })).toBeVisible();
	await expect.element(page.getByText('Configure Static OAuth', { exact: true })).not.toBeVisible();
	expect(vi.mocked(goto)).toHaveBeenCalledTimes(1);
	expectOAuthRedirectDropped();
}

describe('StaticOAuthConfigureModal', () => {
	beforeEach(() => {
		setOAuthRedirect();
		vi.mocked(goto).mockClear();
	});

	afterEach(() => {
		if (originalUrl) {
			Object.defineProperty(appPage, 'url', originalUrl);
		} else {
			Reflect.deleteProperty(appPage, 'url');
		}
	});

	it.each(returnPrompts)(
		'saves trimmed credentials and returns to $name when Go Back is accepted',
		async ({ redirect, destination, prompt }) => {
			const save = deferredSave();
			setOAuthRedirect(redirectValue(redirect));
			await openModal(save.onSave);
			await fillAndSave();

			await vi.waitFor(() => expect(save.onSave).toHaveBeenCalledWith(savedCredentials));
			await tick();
			await expect.element(page.getByText('Go Back?', { exact: true })).not.toBeVisible();

			save.succeed();
			await expectReturnPrompt(prompt);
			await page.getByRole('button', { name: 'Go Back', exact: true }).click();

			expect(vi.mocked(goto)).toHaveBeenCalledTimes(2);
			expectOAuthRedirectDropped();
			expect(vi.mocked(goto)).toHaveBeenNthCalledWith(2, destination);
		}
	);

	it('saves credentials and stays on the server when Skip is chosen', async () => {
		const onSave = vi.fn().mockResolvedValue(undefined);
		setOAuthRedirect(workflowPath);
		await openModal(onSave);
		await fillAndSave();

		await expectReturnPrompt(toolSetupPrompt);
		expect(onSave).toHaveBeenCalledWith(savedCredentials);
		await page.getByRole('button', { name: 'Skip', exact: true }).click();

		await expect.element(page.getByText(toolSetupPrompt, { exact: true })).not.toBeVisible();
		expect(vi.mocked(goto)).toHaveBeenCalledTimes(1);
		expectOAuthRedirectDropped();
	});

	it.each(crossOriginRedirects)(
		'saves credentials and stays on the server when the redirect is $name',
		async ({ redirect }) => {
			const onSave = vi.fn().mockResolvedValue(undefined);
			setOAuthRedirect(redirect());
			await openModal(onSave);
			await fillAndSave();

			await expect
				.element(page.getByText('Configure Static OAuth', { exact: true }))
				.not.toBeVisible();
			expect(onSave).toHaveBeenCalledWith(savedCredentials);
			await tick();
			await expect.element(page.getByText('Go Back?', { exact: true })).not.toBeVisible();
			expect(vi.mocked(goto)).not.toHaveBeenCalled();
		}
	);

	it('keeps the configuration dialog open when saving credentials fails', async () => {
		const onSave = vi.fn().mockRejectedValue(new Error('Credential service unavailable'));
		setOAuthRedirect(workflowPath);
		await openModal(onSave);
		await fillAndSave();

		await expect.element(page.getByText('Credential service unavailable')).toBeVisible();
		await expect
			.element(page.getByRole('heading', { name: 'Configure Static OAuth' }))
			.toBeVisible();
		expect(onSave).toHaveBeenCalledWith(savedCredentials);
		await tick();
		await expect.element(page.getByText('Go Back?', { exact: true })).not.toBeVisible();
		expect(vi.mocked(goto)).not.toHaveBeenCalled();
	});

	it('saves credentials without a return prompt when the page has no oauth redirect', async () => {
		const onSave = vi.fn().mockResolvedValue(undefined);
		await openModal(onSave);
		await fillAndSave();

		await expect
			.element(page.getByText('Configure Static OAuth', { exact: true }))
			.not.toBeVisible();
		expect(onSave).toHaveBeenCalledWith(savedCredentials);
		await tick();
		await expect.element(page.getByText('Go Back?', { exact: true })).not.toBeVisible();
		expect(vi.mocked(goto)).not.toHaveBeenCalled();
	});
});

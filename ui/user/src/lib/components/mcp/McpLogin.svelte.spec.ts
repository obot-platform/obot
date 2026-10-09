import McpLogin from './McpLogin.svelte';
import { expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

it('shows a copyable command for the specific UI attempt', async () => {
	await render(McpLogin, {
		url: 'https://obot.example/oauth/mcp/login/test'
	});
	await expect
		.element(page.getByLabelText('Authentication command'))
		.toHaveTextContent("obot mcp login --url 'https://obot.example/oauth/mcp/login/test'");
	await expect.element(page.getByRole('button', { name: /Copy/ })).toBeVisible();
});

it('quotes shell metacharacters', async () => {
	await render(McpLogin, {
		url: "https://obot.example/oauth/mcp/login/test?name=a'b&x=$(echo)"
	});
	await expect
		.element(page.getByLabelText('Authentication command'))
		.toHaveTextContent(
			`obot mcp login --url 'https://obot.example/oauth/mcp/login/test?name=a'"'"'b&x=$(echo)'`
		);
});

it('resolves relative attempt URLs and continues only when requested', async () => {
	const onComplete = vi.fn();
	await render(McpLogin, { url: '/oauth/mcp/login/attempt', onComplete });
	await expect
		.element(page.getByLabelText('Authentication command'))
		.toHaveTextContent(`obot mcp login --url '${window.location.origin}/oauth/mcp/login/attempt'`);
	expect(onComplete).not.toHaveBeenCalled();
	await page.getByRole('button', { name: 'Continue', exact: true }).click();
	expect(onComplete).toHaveBeenCalledOnce();
});

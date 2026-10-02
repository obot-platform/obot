import McpLogin from './McpLogin.svelte';
import { expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

it('shows a copyable command with deduplicated custom callback paths', async () => {
	await render(McpLogin, {
		url: 'https://obot.example/mcp-connect/test',
		callbackPaths: ['/custom/callback', '/custom/callback', '/oauth/callback']
	});
	await expect
		.element(page.getByLabelText('Authentication command'))
		.toHaveTextContent(
			"obot mcp login --url 'https://obot.example/mcp-connect/test' --callback-path '/custom/callback' --callback-path '/oauth/callback'"
		);
	await expect.element(page.getByRole('button', { name: /Copy/ })).toBeVisible();
});

it('quotes shell metacharacters and omits the default callback flag', async () => {
	await render(McpLogin, {
		url: "https://obot.example/mcp-connect/test?name=a'b&x=$(echo)",
		callbackPaths: ['/oauth/callback']
	});
	await expect
		.element(page.getByLabelText('Authentication command'))
		.toHaveTextContent(
			`obot mcp login --url 'https://obot.example/mcp-connect/test?name=a'"'"'b&x=$(echo)'`
		);
});

import { m, locales } from '$lib/i18n';
import { getLocale, overwriteGetLocale } from '$lib/paraglide/runtime';
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

it.each(locales)(
	'renders localized login instructions and actions in %s on mobile',
	async (locale) => {
		const restoreLocale = getLocale;
		overwriteGetLocale(() => locale);
		await page.viewport(390, 844);
		try {
			const onComplete = vi.fn();
			await render(McpLogin, { url: 'https://obot.example/oauth/mcp/login/test', onComplete });
			await expect
				.element(page.getByText(m.mcps_login_instructions(), { exact: true }))
				.toBeVisible();
			await expect
				.element(page.getByText(m.mcps_login_retry_instructions(), { exact: true }))
				.toBeVisible();
			await expect.element(page.getByRole('link', { name: m.mcps_login_install() })).toBeVisible();
			const command = page.getByLabelText(m.mcps_login_command());
			await expect
				.element(command)
				.toHaveTextContent("obot mcp login --url 'https://obot.example/oauth/mcp/login/test'");
			const button = page.getByRole('button', { name: m.mcps_login_continue(), exact: true });
			await expect.element(button).toBeVisible();
			expect(command.element().getBoundingClientRect().right).toBeLessThanOrEqual(390);
			expect(button.element().getBoundingClientRect().right).toBeLessThanOrEqual(390);
			await button.click();
			expect(onComplete).toHaveBeenCalledOnce();
		} finally {
			overwriteGetLocale(restoreLocale);
			await page.viewport(1280, 720);
		}
	}
);

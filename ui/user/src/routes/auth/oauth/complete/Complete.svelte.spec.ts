import * as navigation from '$lib/navigation';
import { onOAuthCompletion } from '$lib/services/mcp/oauth-completion';
import Complete from './Complete.svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock(import('$lib/navigation'), { spy: true });

describe('OAuth complete page', () => {
	let stop: (() => void) | undefined;
	afterEach(() => {
		stop?.();
		vi.mocked(navigation.navigateTo).mockRestore();
	});

	function listen() {
		const announced = vi.fn();
		stop = onOAuthCompletion(announced, 0);
		return announced;
	}

	it('tells the waiting page that a login finished in this tab', async () => {
		const announced = listen();
		render(Complete, { redirectURL: '' });

		await expect.element(page.getByText('You can now close this window.')).toBeVisible();
		await vi.waitFor(() => expect(announced).toHaveBeenCalledOnce());
	});

	it('does not announce when it is handing the login back to the client', async () => {
		vi.mocked(navigation.navigateTo).mockImplementation(() => {});
		const announced = listen();
		render(Complete, { redirectURL: '/oauth/complete/request-1' });

		await expect
			.element(page.getByRole('heading', { name: 'Authentication Complete' }))
			.toBeVisible();
		await expect.element(page.getByText('You can now close this window.')).not.toBeInTheDocument();
		await vi.waitFor(() =>
			expect(navigation.navigateTo).toHaveBeenCalledWith('/oauth/complete/request-1')
		);
		expect(announced).not.toHaveBeenCalled();
	});
});

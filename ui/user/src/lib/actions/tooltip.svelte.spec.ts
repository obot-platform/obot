import InfoTooltip from '$lib/components/InfoTooltip.svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function tooltipHost() {
	return page.getByCSS('.tooltip-portal-daisy-host');
}

function setTheme(theme: string) {
	document.documentElement.setAttribute('data-theme', theme);
}

afterEach(() => {
	setTheme('nanobotlight');
});

describe('tooltip action', () => {
	it('follows the app theme after it switches', async () => {
		setTheme('nanobotlight');
		render(InfoTooltip, { text: 'Server details' });

		await expect.element(tooltipHost()).toHaveAttribute('data-theme', 'nanobotlight');

		setTheme('nanobotdark');
		await expect.element(tooltipHost()).toHaveAttribute('data-theme', 'nanobotdark');

		setTheme('nanobotlight');
		await expect.element(tooltipHost()).toHaveAttribute('data-theme', 'nanobotlight');
	});

	it('settles instead of reacting to its own theme writes', async () => {
		setTheme('nanobotlight');
		render(InfoTooltip, { text: 'Server details' });
		await expect.element(tooltipHost()).toBeInTheDocument();

		const host = (await tooltipHost().element()) as HTMLElement;
		let writes = 0;
		const observer = new MutationObserver((records) => {
			writes += records.length;
		});
		observer.observe(host, { attributes: true, attributeFilter: ['data-theme'] });

		try {
			setTheme('nanobotdark');
			await vi.waitFor(() => expect(host.getAttribute('data-theme')).toBe('nanobotdark'));
			// A feedback loop would keep appending records for as long as we wait.
			await new Promise((resolve) => setTimeout(resolve, 50));
			expect(writes).toBe(1);
		} finally {
			observer.disconnect();
		}
	});
});

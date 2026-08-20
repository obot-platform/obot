import InfoTooltip from './InfoTooltip.svelte';
import { createRawSnippet } from 'svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function tooltipHost() {
	return page.getByCSS('.tooltip-portal-daisy-host');
}

describe('InfoTooltip.svelte', () => {
	it('exposes the help text as the accessible name of a button', async () => {
		render(InfoTooltip, { text: 'Server details' });

		const trigger = page.getByRole('button', { name: 'Server details' });
		await expect.element(trigger).toBeInTheDocument();
		await expect.element(tooltipHost()).toHaveAttribute('role', 'tooltip');
		await expect.element(tooltipHost()).toHaveAttribute('aria-hidden', 'true');
	});

	it('shows the tooltip for keyboard focus and hides it again on Escape', async () => {
		render(InfoTooltip, { text: 'Server details' });

		const trigger = page.getByRole('button', { name: 'Server details' });
		(await trigger.element()).focus();

		await expect.element(tooltipHost()).toHaveAttribute('aria-hidden', 'false');

		(await trigger.element()).dispatchEvent(
			new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })
		);

		await expect.element(tooltipHost()).toHaveAttribute('aria-hidden', 'true');
	});

	it('uses a generic name when the tooltip is snippet content', async () => {
		const children = createRawSnippet(() => ({
			render: () => '<span>Longer help copy</span>'
		}));
		render(InfoTooltip, { children });

		await expect
			.element(page.getByRole('button', { name: 'More information' }))
			.toBeInTheDocument();
	});

	it('prefers an explicit ariaLabel over the tooltip text', async () => {
		render(InfoTooltip, { text: 'Server details', ariaLabel: 'About this server' });

		await expect
			.element(page.getByRole('button', { name: 'About this server' }))
			.toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: 'Server details' }))
			.not.toBeInTheDocument();
	});

	it('renders nothing when there is no tooltip content', async () => {
		render(InfoTooltip, { text: '   ' });

		await expect.element(page.getByRole('button')).not.toBeInTheDocument();
	});
});

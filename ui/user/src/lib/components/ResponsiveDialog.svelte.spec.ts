import ResponsiveDialog from './ResponsiveDialog.svelte';
import { createRawSnippet } from 'svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const children = createRawSnippet(() => ({
	render: () => '<p>Dialog body</p>'
}));

async function renderDialog(props: Record<string, unknown> = {}) {
	const result = await render(ResponsiveDialog, {
		children,
		title: 'Server details',
		animate: null,
		...props
	});
	await result.component.open();
	return result;
}

function dialogElement() {
	return page.getByCSS('dialog.dialog').element() as HTMLDialogElement;
}

describe('ResponsiveDialog.svelte', () => {
	it('opens modal and spans the full viewport by default', async () => {
		await renderDialog();

		await expect.element(page.getByText('Dialog body')).toBeInTheDocument();

		const dialog = dialogElement();
		expect(dialog.matches(':modal')).toBe(true);
		expect(dialog.style.right).toBe('');
		expect(dialog.dataset.nonModal).toBeUndefined();
	});

	it('insets itself and stays non-modal when a right panel must remain interactive', async () => {
		await renderDialog({ rightPanelWidth: 320 });

		await expect.element(page.getByText('Dialog body')).toBeInTheDocument();

		const dialog = dialogElement();
		// Non-modal keeps content beside the dialog interactive instead of inert.
		expect(dialog.matches(':modal')).toBe(false);
		expect(dialog.open).toBe(true);
		expect(dialog.style.right).toContain('320px');
		expect(dialog.getBoundingClientRect().width).toBeCloseTo(window.innerWidth - 320, 0);
	});
});

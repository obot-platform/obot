import OpenDialogHost from './OpenDialogHost.svelte';
import type { Component, ComponentProps } from 'svelte';
import { expect } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

type Openable = { open: () => void };

/**
 * Renders a dialog component and calls its exported `open()` on mount.
 * Use for components that keep a closed `<dialog>` until `open()` is invoked.
 */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export async function renderOpenDialog<C extends Component<any, Openable>>(
	component: C,
	dialogProps: ComponentProps<C>
) {
	render(OpenDialogHost, { dialog: component, dialogProps });

	const dialog = page.getByRole('dialog');
	await expect.element(dialog).toBeVisible();
	return dialog;
}

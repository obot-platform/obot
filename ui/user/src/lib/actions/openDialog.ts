const GUIDE_PANEL_ID = 'quick-start-guide-panel';

/** Width of the open Quick Start guide panel, or 0 when closed. */
export function getOpenGuidePanelWidth(): number {
	const el = document.getElementById(GUIDE_PANEL_ID);
	if (el?.matches(':popover-open')) {
		return el.getBoundingClientRect().width;
	}
	const raw = getComputedStyle(document.documentElement)
		.getPropertyValue('--guide-panel-width')
		.trim();
	if (!raw) return 0;
	const parsed = Number.parseFloat(raw);
	return Number.isFinite(parsed) ? parsed : 0;
}

/** True when an app dialog should avoid showModal so the guide panel stays interactive. */
export function shouldOpenDialogNonModal(dialog: HTMLDialogElement) {
	const guidePanel = document.getElementById(GUIDE_PANEL_ID);
	return Boolean(guidePanel?.matches(':popover-open') && !guidePanel.contains(dialog));
}

/**
 * Opens a dialog, preferring non-modal `show()` when the Quick Start guide panel is open.
 *
 * Modal dialogs enter the top layer with a full-viewport `::backdrop` and make the rest of
 * the document inert — which blocks the guide panel even when the dialog box is visually
 * inset. Non-modal dialogs stay out of that stack so the guide remains interactive.
 *
 * Dialogs rendered inside the guide panel still use `showModal()` so they stack above it.
 *
 * Prefer this for dialogs that do not use `dialogAnimation` (which applies the same rule
 * when `showModal()` is called). Calling `showModal()` on an animated dialog is enough.
 */
export function openDialog(dialog: HTMLDialogElement) {
	if (shouldOpenDialogNonModal(dialog)) {
		dialog.show();
		return;
	}
	dialog.showModal();
}

/**
 * Browser navigation helpers. Prefer these over calling `window.location` directly
 * so vitest browser-mode tests can spy/mock navigation without patching platform globals.
 */
export function reloadPage() {
	window.location.reload();
}

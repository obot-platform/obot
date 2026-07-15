import { tick } from 'svelte';

export async function highlightFirstAvailableField(
	fields: Record<string, boolean>,
	ids: Record<string, string>
) {
	await tick();
	for (const field of Object.keys(fields)) {
		const elementId = ids[field as keyof typeof ids] ?? field;
		const el =
			(elementId ? document.getElementById(elementId) : null) ??
			(document.querySelector(`[name="${field}"]`) as HTMLElement | null);
		if (el) {
			el.focus();
			break;
		}
	}
}

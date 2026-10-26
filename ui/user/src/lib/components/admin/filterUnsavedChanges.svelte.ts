import { beforeNavigate } from '$app/navigation';
import { m } from '$lib/i18n';
import { onMount } from 'svelte';

export function filterUnsavedChanges(isDirty: () => boolean) {
	let leaving = false;
	function discard() {
		if (isDirty() && !window.confirm(m.mcps_filters_discard_changes())) return false;
		leaving = true;
		return true;
	}
	beforeNavigate(({ cancel, willUnload }) => {
		// Full-page exits are handled once by the native beforeunload warning below.
		if (willUnload) return;
		if (!leaving && isDirty() && !window.confirm(m.mcps_filters_discard_changes())) cancel();
	});
	onMount(() => {
		const handler = (event: BeforeUnloadEvent) => {
			if (!leaving && isDirty()) {
				event.preventDefault();
				event.returnValue = '';
			}
		};
		window.addEventListener('beforeunload', handler);
		return () => window.removeEventListener('beforeunload', handler);
	});
	return {
		discard,
		allowNavigation: () => {
			leaving = true;
		}
	};
}

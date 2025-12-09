<script lang="ts">
	import { PortalHubController } from './hubController.svelte';

	let { id, children } = $props();

	const hub = PortalHubController.get();

	const portal = hub?.portal(id);

	function proxy(...args: unknown[]) {
		portal?.share();

		return children?.(...args);
	}
</script>

{#if portal}
	{@render proxy({ portal })}
{/if}

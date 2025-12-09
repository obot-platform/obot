<script lang="ts">
	import { PortalHubController } from './hubController.svelte';

	let { children } = $props();

	new PortalHubController(() => ({})).share();
</script>

{@render children?.()}

<script lang="ts">
	import { SidebarOpen } from 'lucide-svelte';
	import { context } from '$lib/stores';
</script>

{#if !context.sidebarOpen}
	<button class="icon-button" onclick={() => (context.sidebarOpen = !context.sidebarOpen)}>
		<SidebarOpen />
	</button>
{/if}

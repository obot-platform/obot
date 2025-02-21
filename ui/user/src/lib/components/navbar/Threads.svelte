<script lang="ts">
	import { MessageCirclePlus, SidebarOpen } from 'lucide-svelte';
	import { getLayout } from '$lib/context/layout.svelte';

	const layout = getLayout();
</script>

{#if !layout.threadsOpen}
	<button class="icon-button" onclick={() => (layout.threadsOpen = !layout.threadsOpen)}>
		<SidebarOpen class="icon-default" />
	</button>
	<button class="icon-button" onclick={() => (layout.threadsOpen = !layout.threadsOpen)}>
		<MessageCirclePlus class="icon-default" />
	</button>
{/if}

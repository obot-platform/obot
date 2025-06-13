<script lang="ts">
	import { Pencil, X } from 'lucide-svelte';
	import { getLayout } from '$lib/context/chatLayout.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';

	const layout = getLayout();
</script>

{#if layout.projectEditorOpen}
	<button
		data-testid="obot-editor-btn"
		onclick={() => {
			layout.projectEditorOpen = false;
		}}
		use:tooltip={'Close Advanced Editor'}
		class="border-blue bg-blue relative flex items-center rounded-full border p-2 text-xs text-white transition-[background-color] duration-200"
	>
		<X class="size-5" />
	</button>
{:else}
	<button
		data-testid="obot-editor-btn"
		use:tooltip={'Advanced Editor'}
		onclick={() => {
			layout.projectEditorOpen = true;
			layout.sidebarOpen = false;
		}}
		class="text-gray hover:bg-surface3 border-surface3 relative flex items-center rounded-full border bg-transparent p-2 text-xs opacity-50 transition-[background-color] duration-200 hover:opacity-100"
	>
		<Pencil class="size-5" />
	</button>
{/if}

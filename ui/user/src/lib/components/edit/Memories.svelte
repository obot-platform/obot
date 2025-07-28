<script lang="ts">
	import { type Project } from '$lib/services';
	import MemoryContent from '$lib/components/MemoriesDialog.svelte';
	import { RefreshCcw } from 'lucide-svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { getLayout } from '$lib/context/chatLayout.svelte';

	interface Props {
		project: Project;
	}

	let { project = $bindable() }: Props = $props();
	let memories = $state<ReturnType<typeof MemoryContent>>();
	const layout = getLayout();
</script>

<div class="flex flex-col text-xs">
	<div class="flex items-center justify-between">
		<p class="text-md grow font-medium">Memories</p>
		<div class="flex items-center">
			{#if layout.sidebarMemoryUpdateAvailable}
				<button
					class="p-2 text-gray-400 transition-colors duration-200 hover:text-black dark:text-gray-600 dark:hover:text-white"
					onclick={() => {
						memories?.refresh();
						layout.sidebarMemoryUpdateAvailable = false;
					}}
					use:tooltip={'Refresh Memories'}
				>
					<RefreshCcw class="size-4" />
				</button>
			{/if}
		</div>
	</div>
	<div class="pt-2">
		<MemoryContent bind:this={memories} {project} showPreview />
	</div>
</div>

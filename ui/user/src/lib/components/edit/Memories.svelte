<script lang="ts">
	import { type Project } from '$lib/services';
	import MemoryContent from '$lib/components/MemoriesDialog.svelte';
	import { RefreshCcw, View } from 'lucide-svelte';
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
					class="icon-button"
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

<script lang="ts">
	import { type Project } from '$lib/services';
	import MemoryContent from '$lib/components/MemoriesDialog.svelte';
	import { RefreshCcw, View } from 'lucide-svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';

	interface Props {
		project: Project;
	}

	let { project = $bindable() }: Props = $props();
	let memories = $state<ReturnType<typeof MemoryContent>>();
</script>

<div class="flex flex-col text-xs">
	<div class="flex items-center justify-between">
		<p class="text-md grow font-medium">Memories</p>
		<div class="flex items-center">
			<button
				class="icon-button"
				onclick={() => memories?.refresh()}
				use:tooltip={'Refresh Memories'}
			>
				<RefreshCcw class="size-4" />
			</button>
			<button
				class="icon-button"
				onclick={() => memories?.viewAllMemories()}
				use:tooltip={'View All Memories'}
			>
				<View class="size-5" />
			</button>
		</div>
	</div>
	<div>
		<MemoryContent bind:this={memories} {project} showPreview />
	</div>
</div>

<script lang="ts">
	import { type Project } from '$lib/services';
	import MemoryContent from '$lib/components/MemoriesDialog.svelte';

	interface Props {
		project: Project;
	}

	let { project = $bindable() }: Props = $props();
	let memories = $state<ReturnType<typeof MemoryContent>>();
</script>

<div class="flex flex-col gap-2">
	<h2 class="text-xl font-semibold">Memories</h2>
	<MemoryContent bind:this={memories} {project} showPreview />
</div>

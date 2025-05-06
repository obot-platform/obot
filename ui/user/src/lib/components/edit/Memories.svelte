<script lang="ts">
	import { type Project } from '$lib/services';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { HELPER_TEXTS } from '$lib/context/helperMode.svelte';
	import MemoryContent from '$lib/components/MemoriesDialog.svelte';

	interface Props {
		project: Project;
	}

	let { project = $bindable() }: Props = $props();
</script>

<CollapsePane
	classes={{ header: 'pl-3 py-2 text-md', content: 'p-2 font-light' }}
	iconSize={5}
	header="Memories"
	helpText={HELPER_TEXTS.memories}
>
	<MemoryContent {project} showPreview />
</CollapsePane>

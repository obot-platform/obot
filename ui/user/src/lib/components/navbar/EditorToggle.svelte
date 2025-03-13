<script lang="ts">
	import { Pencil } from 'lucide-svelte';
	import { getLayout } from '$lib/context/layout.svelte';
	import type { Project } from '$lib/services';

	interface Props {
		project: Project;
	}

	const layout = getLayout();

	let { project }: Props = $props();
</script>

<button
	onclick={() => {
		if (project.editor) {
			layout.projectEditorOpen = true;
		}
	}}
	class="group relative mr-2 flex items-center rounded-full bg-transparent p-2 text-xs text-gray transition-[background-color] duration-200 hover:bg-blue hover:px-4 hover:text-white active:bg-blue-700"
>
	<Pencil class="h-5 w-5" />
	<span
		class="w-0 overflow-hidden transition-[width] duration-300 group-hover:ml-2 group-hover:w-auto"
	>
		<span
			class="delay-250 inline-block translate-x-full transition-[transform] duration-300 group-hover:translate-x-0"
			>{project.editor ? 'Edit Obot' : 'Clone Obot'}</span
		>
	</span>
</button>

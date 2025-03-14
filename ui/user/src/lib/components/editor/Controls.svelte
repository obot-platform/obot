<script lang="ts">
	import { X, Download } from 'lucide-svelte';
	import { EditorService, type Project } from '$lib/services';
	import { term } from '$lib/stores';
	import { getLayout } from '$lib/context/layout.svelte';
	import { twMerge } from 'tailwind-merge';
	import Files from '../edit/Files.svelte';

	interface Props {
		navBar?: boolean;
		project: Project;
		class?: string;
		currentThreadID?: string;
	}

	let { navBar = false, project, class: className, currentThreadID }: Props = $props();

	const layout = getLayout();
	let show = $derived(navBar || layout.items.length <= 1);
</script>

{#if show}
	<div class={twMerge('flex items-start', className)}>
		{#if currentThreadID}
			<Files {project} thread {currentThreadID} primary={false} />
		{/if}

		<button
			class="icon-button"
			onclick={() => {
				layout.fileEditorOpen = false;
				term.open = false;
			}}
		>
			<X class="h-5 w-5" />
		</button>
	</div>
{/if}

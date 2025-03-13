<script lang="ts">
	import { X, Download } from 'lucide-svelte';
	import { EditorService, type Project } from '$lib/services';
	import { term } from '$lib/stores';
	import { getLayout } from '$lib/context/layout.svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		navBar?: boolean;
		project: Project;
		class?: string;
	}

	let { navBar = false, project, class: className }: Props = $props();

	const layout = getLayout();
	let show = $derived(navBar || layout.items.length <= 1);
	let downloadable = $derived.by(() => {
		const selected = layout.items.find((item) => item.selected);
		return !!selected?.file;
	});
</script>

{#if show}
	<div class={twMerge('flex items-start', className)}>
		{#if downloadable}
			<button
				class="icon-button"
				onclick={() => {
					const selected = layout.items.find((item) => item.selected);
					if (selected) {
						EditorService.download(layout.items, project, selected.name, {
							taskID: selected.file?.taskID,
							runID: selected.file?.runID,
							threadID: selected.file?.threadID
						});
					}
				}}
			>
				<Download class="h-5 w-5" />
			</button>
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

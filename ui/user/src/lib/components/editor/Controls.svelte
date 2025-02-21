<script lang="ts">
	import { X, Download } from 'lucide-svelte';
	import { EditorService, type Project } from '$lib/services';
	import { term } from '$lib/stores';
	import type { EditorItem } from '$lib/services/editor/index.svelte';
	import { getLayout } from '$lib/context/layout.svelte';

	interface Props {
		navBar?: boolean;
		items: EditorItem[];
		project: Project;
	}

	let { navBar = false, project, items = $bindable() }: Props = $props();

	let show = $derived(navBar || items.length <= 1);
	const layout = getLayout();
	let downloadable = $derived.by(() => {
		const selected = items.find((item) => item.selected);
		return !!selected?.file;
	});
</script>

{#if show}
	<div class="flex">
		{#if downloadable}
			<button
				class="icon-button"
				onclick={() => {
					const selected = items.find((item) => item.selected);
					if (selected) {
						EditorService.download(items, project, selected.name, {
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

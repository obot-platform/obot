<script lang="ts">
	import { Maximize, X, Download, Columns2 } from 'lucide-svelte';
	import { EditorService } from '$lib/services';
	import { term, currentAssistant } from '$lib/stores';

	interface Props {
		navBar?: boolean;
	}

	let editorMaxSize = EditorService.maxSize;
	let { navBar = false }: Props = $props();

	let show = $derived(navBar || EditorService.items.length <= 1);
	let downloadable = $derived.by(() => {
		const selected = EditorService.items.find((item) => item.selected);
		return selected && !selected.table && !selected.task;
	});
</script>

{#if show}
	<div class="flex">
		{#if $editorMaxSize}
			<button
				class="icon-button hidden md:block"
				onclick={() => {
					editorMaxSize.set(false);
				}}
			>
				<Columns2 class="h-5 w-5" />
			</button>
		{:else}
			<button
				class="icon-button hidden md:block"
				onclick={() => {
					editorMaxSize.set(true);
				}}
			>
				<Maximize class="h-5 w-5" />
			</button>
		{/if}
		{#if downloadable}
			<button
				class="icon-button"
				onclick={() => {
					const selected = EditorService.items.find((item) => item.selected);
					if (selected) {
						EditorService.download($currentAssistant.id, selected.name, {
							taskID: selected.taskID,
							runID: selected.runID
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
				EditorService.maxSize.set(false);
				EditorService.visible.set(false);
				term.open = false;
			}}
		>
			<X class="h-5 w-5" />
		</button>
	</div>
{/if}

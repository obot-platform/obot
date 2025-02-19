<script lang="ts">
	import { X, Download } from 'lucide-svelte';
	import { EditorService } from '$lib/services';
	import { term } from '$lib/stores';

	interface Props {
		navBar?: boolean;
	}

	let { navBar = false }: Props = $props();

	let show = $derived(navBar || EditorService.items.length <= 1);
	let downloadable = $derived.by(() => {
		const selected = EditorService.items.find((item) => item.selected);
		return selected && !selected.table && !selected.task && !selected.generic;
	});
</script>

{#if show}
	<div class="flex">
		{#if downloadable}
			<button
				class="icon-button"
				onclick={() => {
					const selected = EditorService.items.find((item) => item.selected);
					if (selected) {
						EditorService.download(selected.name, {
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
				EditorService.setVisible(false);
				term.open = false;
			}}
		>
			<X class="h-5 w-5" />
		</button>
	</div>
{/if}

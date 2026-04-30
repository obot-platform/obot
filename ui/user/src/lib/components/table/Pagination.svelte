<script lang="ts">
	import { ChevronsLeft, ChevronsRight } from 'lucide-svelte';

	interface Props {
		pageIndex: number;
		lastPageIndex: number;
		total: number;
		loading?: boolean;
		itemLabelSingular?: string;
		onPageChange: (idx: number) => void;
	}

	let {
		pageIndex,
		lastPageIndex,
		total,
		loading = false,
		itemLabelSingular,
		onPageChange
	}: Props = $props();
</script>

<div class="flex items-center justify-center gap-4 pt-2">
	<button
		class="button-text flex items-center gap-1 text-xs"
		disabled={pageIndex === 0 || loading}
		onclick={() => onPageChange(pageIndex - 1)}
	>
		<ChevronsLeft class="size-4" /> Previous
	</button>
	<p class="text-muted-content text-xs">
		{pageIndex + 1} of {lastPageIndex + 1}{#if itemLabelSingular}
			· {total}
			{itemLabelSingular}{total === 1 ? '' : 's'}{/if}
	</p>
	<button
		class="button-text flex items-center gap-1 text-xs"
		disabled={pageIndex >= lastPageIndex || loading}
		onclick={() => onPageChange(pageIndex + 1)}
	>
		Next <ChevronsRight class="size-4" />
	</button>
</div>

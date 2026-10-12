<script lang="ts">
	import { m } from '$lib/i18n';
	import { ChevronsLeft, ChevronsRight } from '@lucide/svelte';

	interface Props {
		pageIndex: number;
		lastPageIndex: number;
		total: number;
		loading?: boolean;
		// Names what is paged, so that the buttons of several pagers on a page are told apart.
		label?: string;
		/** Formats the item count shown after the page number, e.g. `(n) => m.devices_count({ count: n })`. */
		itemCountLabel?: (count: number) => string;
		onPageChange: (idx: number) => void;
	}

	let {
		pageIndex,
		lastPageIndex,
		total,
		loading = false,
		label,
		itemCountLabel,
		onPageChange
	}: Props = $props();
</script>

<div class="flex items-center justify-center gap-4 pt-2">
	<button
		class="button-text flex items-center gap-1 text-xs disabled:cursor-default disabled:opacity-50"
		disabled={pageIndex === 0 || loading}
		aria-label={label ? m.core_page_previous_of({ label }) : undefined}
		onclick={() => onPageChange(pageIndex - 1)}
	>
		<ChevronsLeft class="size-4" />
		{m.core_previous()}
	</button>
	<p class="text-muted-content text-xs">
		{m.core_page_of({ page: pageIndex + 1, total: lastPageIndex + 1 })}{#if itemCountLabel}
			· {itemCountLabel(total)}{/if}
	</p>
	<button
		class="button-text flex items-center gap-1 text-xs disabled:cursor-default disabled:opacity-50"
		disabled={pageIndex >= lastPageIndex || loading}
		aria-label={label ? m.core_page_next_of({ label }) : undefined}
		onclick={() => onPageChange(pageIndex + 1)}
	>
		{m.core_next()}
		<ChevronsRight class="size-4" />
	</button>
</div>

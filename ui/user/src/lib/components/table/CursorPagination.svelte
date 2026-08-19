<script lang="ts">
	import { ChevronsLeft, ChevronsRight } from '@lucide/svelte';

	/**
	 * Previous/next pager for listings that page by cursor.
	 *
	 * Unlike `Pagination.svelte`, this one has no total and no last page: a cursor-paged source
	 * knows only whether another page exists, so the page count cannot be shown and an arbitrary
	 * page cannot be jumped to.
	 */
	interface Props {
		pageIndex: number;
		hasPrevious: boolean;
		hasNext: boolean;
		loading?: boolean;
		onPrevious: () => void;
		onNext: () => void;
	}

	let { pageIndex, hasPrevious, hasNext, loading = false, onPrevious, onNext }: Props = $props();
</script>

<div class="flex items-center justify-center gap-4 pt-2">
	<button
		class="button-text flex items-center gap-1 text-xs disabled:cursor-default disabled:opacity-50"
		disabled={!hasPrevious || loading}
		onclick={onPrevious}
	>
		<ChevronsLeft class="size-4" /> Previous
	</button>
	<p class="text-muted-content text-xs">
		Page {pageIndex + 1}
	</p>
	<button
		class="button-text flex items-center gap-1 text-xs disabled:cursor-default disabled:opacity-50"
		disabled={!hasNext || loading}
		onclick={onNext}
	>
		Next <ChevronsRight class="size-4" />
	</button>
</div>

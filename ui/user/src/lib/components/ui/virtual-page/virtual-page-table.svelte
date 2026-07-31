<script module lang="ts">
	export type VirtualListViewportProps<T> = {
		class?: string;
		header?: Snippet;
		children: Snippet<
			[
				{
					items: { index: number; data: T }[];
				}
			]
		>;
	};
</script>

<script lang="ts" generics="T">
	import { calculateStickyTop, findScrollContainer } from '$lib/components/table/stickyOffset';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { getVirtualPageContext, type VirtualPageContext } from './context';
	import { onMount, type Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	const context: VirtualPageContext<T> | undefined = getVirtualPageContext();

	const top = $derived(context?.top ?? 0);
	const bottom = $derived(context?.bottom ?? 0);
	const rows = $derived(context?.rows ?? []);

	if (!context) {
		throw new Error('VirtualPageTable must be used within a VirtualPageRoot');
	}

	let { class: klass = '', children, header, ...restProps }: VirtualListViewportProps<T> = $props();

	let wrapperRef: HTMLDivElement | undefined = $state();
	let sentinelRef: HTMLDivElement | undefined = $state();
	let headerScrollRef: HTMLDivElement | undefined = $state();
	let bodyScrollRef: HTMLDivElement | undefined = $state();
	let headerTableRef: HTMLTableElement | undefined = $state();
	let bodyTableRef: HTMLTableElement | undefined = $state();

	let stickyTop = $state(0);
	let isStuck = $state(false);
	let colWidths = $state<number[]>([]);
	let headerLabels = $state<string[]>([]);
	let isScrolling = false;

	const columnCount = $derived(
		Math.max(1, colWidths.length || (headerTableRef?.tHead?.rows[0]?.cells.length ?? 1))
	);

	const tableClass = $derived(twMerge('h-min w-full border-collapse', klass));
	// Header width is driven by <th> (incl. resize); body mirrors measured widths via colgroup.
	const headerTableStyle = 'table-layout: fixed; min-width: 100%;';
	const bodyTableStyle = $derived(
		colWidths.length > 0
			? `table-layout: fixed; min-width: 100%; width: ${colWidths.reduce((a, b) => a + b, 0)}px;`
			: 'table-layout: fixed; width: 100%;'
	);

	function syncScroll(source: HTMLDivElement, target: HTMLDivElement) {
		if (isScrolling) return;
		isScrolling = true;
		target.scrollLeft = source.scrollLeft;
		requestAnimationFrame(() => {
			isScrolling = false;
		});
	}

	function measureColumnWidths() {
		const cells = headerTableRef?.tHead?.rows[0]?.cells;
		if (!cells?.length) return;
		colWidths = Array.from(cells).map((cell) => cell.getBoundingClientRect().width);
		// Labels for the body table's visually-hidden thead (associates columns for AT).
		headerLabels = Array.from(cells).map((cell) =>
			(cell.textContent ?? '').replace(/\s+/g, ' ').trim()
		);
	}

	function updateStickyTop() {
		if (wrapperRef) {
			stickyTop = calculateStickyTop(wrapperRef);
		}
	}

	onMount(() => {
		if (!headerScrollRef || !bodyScrollRef) return;

		const handleHeaderScroll = () => syncScroll(headerScrollRef!, bodyScrollRef!);
		const handleBodyScroll = () => syncScroll(bodyScrollRef!, headerScrollRef!);

		headerScrollRef.addEventListener('scroll', handleHeaderScroll);
		bodyScrollRef.addEventListener('scroll', handleBodyScroll);

		return () => {
			headerScrollRef?.removeEventListener('scroll', handleHeaderScroll);
			bodyScrollRef?.removeEventListener('scroll', handleBodyScroll);
		};
	});

	onMount(() => {
		const parentContainer = wrapperRef?.closest(
			'.default-scrollbar-thin, .virtual-page-viewport'
		) as HTMLElement | undefined;
		if (!wrapperRef) return;

		let measureRaf: number | undefined;
		let layoutTimeout: ReturnType<typeof setTimeout> | undefined;

		const scheduleColumnMeasure = () => {
			if (measureRaf !== undefined) return;
			measureRaf = requestAnimationFrame(() => {
				measureRaf = undefined;
				measureColumnWidths();
			});
		};

		const debouncedLayoutMeasure = () => {
			scheduleColumnMeasure();
			clearTimeout(layoutTimeout);
			layoutTimeout = setTimeout(updateStickyTop, 100);
		};

		const cellResizeObserver = new ResizeObserver(scheduleColumnMeasure);
		const layoutResizeObserver = new ResizeObserver(debouncedLayoutMeasure);

		if (parentContainer) layoutResizeObserver.observe(parentContainer);
		if (headerTableRef) cellResizeObserver.observe(headerTableRef);

		const headerCells = headerTableRef?.tHead?.rows[0]?.cells;
		if (headerCells) {
			for (const cell of headerCells) {
				cellResizeObserver.observe(cell);
			}
		}

		const initialMeasureTimeout = setTimeout(() => {
			measureColumnWidths();
			updateStickyTop();
		}, PAGE_TRANSITION_DURATION + 50);

		return () => {
			if (measureRaf !== undefined) cancelAnimationFrame(measureRaf);
			clearTimeout(layoutTimeout);
			clearTimeout(initialMeasureTimeout);
			cellResizeObserver.disconnect();
			layoutResizeObserver.disconnect();
		};
	});

	// Drop top rounding while stuck so row content can't show through corner gaps.
	$effect(() => {
		const sentinel = sentinelRef;
		const topOffset = stickyTop;
		if (!sentinel) return;

		const root = findScrollContainer(sentinel);
		const observer = new IntersectionObserver(
			([entry]) => {
				isStuck = !entry.isIntersecting;
			},
			{
				root,
				threshold: 0,
				rootMargin: `-${topOffset + 1}px 0px 0px 0px`
			}
		);
		observer.observe(sentinel);

		return () => observer.disconnect();
	});
</script>

<div bind:this={wrapperRef} data-table-root class="w-full min-w-0">
	<div bind:this={sentinelRef} class="h-px w-full" aria-hidden="true"></div>
	<div
		class={twMerge(
			'dark:bg-base-200 bg-base-300 sticky left-0 z-40 w-full overflow-hidden',
			!isStuck && 'rounded-t-lg'
		)}
		style="top: {stickyTop}px;"
	>
		<div class="sticky-table-header-scroll w-full overflow-x-auto" bind:this={headerScrollRef}>
			<!-- Visual sticky header only; hidden from AT so the body table owns semantics. -->
			<table
				bind:this={headerTableRef}
				class={tableClass}
				style={headerTableStyle}
				aria-hidden="true"
			>
				{@render header?.()}
			</table>
		</div>
	</div>

	<div class="w-full overflow-x-auto rounded-b-lg" bind:this={bodyScrollRef}>
		<table bind:this={bodyTableRef} class={tableClass} style={bodyTableStyle} {...restProps}>
			{#if colWidths.length > 0}
				<colgroup>
					{#each colWidths as width, i (i)}
						<col style="width: {width}px;" />
					{/each}
				</colgroup>
			{/if}

			{#if headerLabels.length > 0}
				<thead class="sr-only">
					<tr>
						{#each headerLabels as label, i (i)}
							<th scope="col">{label}</th>
						{/each}
					</tr>
				</thead>
			{/if}

			<tbody bind:this={context.elements.content}>
				{#if top > 0}
					<tr aria-hidden="true" class="pointer-events-none">
						<td
							colspan={columnCount}
							style="height: {top}px; padding: 0; border: none; line-height: 0;"
						></td>
					</tr>
				{/if}

				{@render children?.({ items: rows })}

				{#if bottom > 0}
					<tr aria-hidden="true" class="pointer-events-none">
						<td
							colspan={columnCount}
							style="height: {bottom}px; padding: 0; border: none; line-height: 0;"
						></td>
					</tr>
				{/if}
			</tbody>
		</table>
	</div>
</div>

<style>
	.sticky-table-header-scroll {
		scrollbar-width: none;
	}
	.sticky-table-header-scroll::-webkit-scrollbar {
		display: none;
	}
</style>

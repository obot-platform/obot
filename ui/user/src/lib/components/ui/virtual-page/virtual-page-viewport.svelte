<script module lang="ts">
	export type VirtualListViewportProps<T> = {
		class?: string;
		as?: string;
		data?: T[];
		itemHeight: number; // Optional for dynamic heights
		overscan?: number; // Buffer items to render above/below viewport
		onScroll?: (scrollTop: number) => void;
		scrollToIndex?: number; // Scroll to specific item
		disabled?: boolean;
		children: Snippet<[{ items: { index: number; data: T }[] }]>;
	};
</script>

<script lang="ts" generics="T">
	import { tick, untrack, type Snippet } from 'svelte';
	import { setVirtualPageContext, type VirtualPageContext } from './context';
	import { twMerge } from 'tailwind-merge';
	import { throttle } from 'es-toolkit';
	import { Render } from '../render';

	let {
		class: klass = '',
		as = 'div',
		data = [],
		itemHeight,
		overscan = 5,
		disabled = false,
		children,
		onScroll,
		scrollToIndex,
		...restProps
	}: VirtualListViewportProps<T> = $props();

	let start = $state(0);
	let end = $state(0);

	const visibleItems = $derived(
		data.slice(start, end).map((d, i) => ({ index: i + start, data: d }))
	);

	let rootElement: HTMLElement | undefined = $state();
	let viewportElement: HTMLElement | undefined = $state();
	let contentElement: HTMLElement | undefined = $state();

	let top = $state(0);
	let bottom = $state(0);

	let viewportHeight = $state(0);

	const context: VirtualPageContext<T> = {
		elements: {
			get viewport() {
				return viewportElement;
			},
			set viewport(el) {
				viewportElement = el;
			},
			get content() {
				return contentElement;
			},
			set content(el) {
				contentElement = el;
			}
		},

		get top() {
			return top;
		},
		set top(value) {
			top = value;
		},

		get bottom() {
			return bottom;
		},
		set bottom(value) {
			bottom = value;
		},

		get overscan() {
			return overscan;
		},
		get itemHeight() {
			return itemHeight;
		},
		get scrollToIndex() {
			return scrollToIndex;
		},

		get disabled() {
			return disabled;
		},
		set disabled(value) {
			disabled = value;
		},

		get height() {
			return viewportHeight;
		},
		get rows() {
			return visibleItems;
		},
		get data() {
			return data;
		},
		set data(value) {
			data = value;
		}
	};

	setVirtualPageContext(context);

	// Height management with exponential moving average
	let heightMap: number[] = $state([]);
	let averageHeight = $state(itemHeight || 50);
	let heightSampleCount = $state(0);

	let rows: HTMLElement[] = $state([]);
	let mounted = $state(false);

	// trigger initial refresh
	$effect(() => {
		if (!contentElement) return;

		rows = Array.from(contentElement.getElementsByClassName('virtual-list-row')) as HTMLElement[];

		mounted = true;

		// Give the browser time to render and measure the viewport
		setTimeout(() => {
			if (untrack(() => viewportElement)) {
				viewportHeight = untrack(() => viewportElement?.offsetHeight ?? 0);
			}

			refresh();
		}, 0);
	});

	// Watch for data changes and viewport size changes
	$effect(() => {
		if (mounted) refresh();
	});

	// Handle scroll to specific index
	$effect(() => {
		if (scrollToIndex !== undefined && viewportElement && mounted) {
			scrollToItem(scrollToIndex);
		}
	});

	const handleScroll = throttle(async () => {
		if (!viewportElement) return;
		if (disabled) return;

		const { scrollTop } = viewportElement;

		// Call user's onScroll callback
		onScroll?.(scrollTop);

		await refresh();
	}, 1000 / 60);

	// Improved height estimation using exponential moving average
	function updateAverageHeight(newHeight: number) {
		if (heightSampleCount < 1) {
			averageHeight = newHeight;
			heightSampleCount = 1;
		} else {
			// Exponential moving average with alpha = 0.1 for stability
			const alpha = Math.min(0.1, 1 / heightSampleCount);
			averageHeight = alpha * newHeight + (1 - alpha) * averageHeight;
			heightSampleCount++;
		}
	}

	function getItemHeight(index: number): number {
		return heightMap[index] || itemHeight || averageHeight;
	}

	// Cache the table element to avoid repeated DOM queries
	let tableElement: HTMLElement | undefined = $state();

	async function refresh() {
		if (!viewportElement || !mounted) return;

		tableElement = contentElement?.closest('table') as HTMLElement;

		const rootOffsetTop = rootElement?.offsetTop ?? 0;
		const contentOffsetTop = tableElement?.offsetTop ?? 0;

		const totalOffsetTop = rootOffsetTop + contentOffsetTop;

		// Calculate visible range with overscan buffer
		const scrollTop = Math.max(0, viewportElement.scrollTop - totalOffsetTop);

		const startIndex = Math.max(0, findStartIndex(scrollTop) - overscan);
		const endIndex = Math.min(data.length, findEndIndex(scrollTop, startIndex) + overscan);

		start = startIndex;
		end = endIndex;

		await tick(); // Wait for DOM update

		// Update height measurements for visible items
		updateHeights();
		updatePadding();
	}

	function findStartIndex(scrollTop: number): number {
		let index = 0;
		let accumulatedHeight = 0;

		while (index < data.length && accumulatedHeight + getItemHeight(index) <= scrollTop) {
			accumulatedHeight += getItemHeight(index);
			index++;
		}

		return index;
	}

	function findEndIndex(scrollTop: number, startIndex: number): number {
		let index = startIndex;
		let accumulatedHeight = 0;

		// Start from the scroll position
		for (let i = 0; i < startIndex; i++) {
			accumulatedHeight += getItemHeight(i);
		}

		// Use a minimum viewport height to ensure we render enough items initially
		const effectiveViewportHeight = Math.max(viewportHeight, 800);

		while (index < data.length && accumulatedHeight <= scrollTop + effectiveViewportHeight) {
			accumulatedHeight += getItemHeight(index);
			index++;
		}

		return index;
	}

	function updateHeights() {
		if (!contentElement) return;

		rows = Array.from(contentElement.getElementsByClassName('virtual-list-row')) as HTMLElement[];

		// Update heights for currently rendered items
		rows.forEach((row, i) => {
			const actualIndex = start + i;
			const measuredHeight = itemHeight || row.offsetHeight;

			if (heightMap[actualIndex] !== measuredHeight) {
				heightMap[actualIndex] = measuredHeight;
				if (!itemHeight) {
					updateAverageHeight(measuredHeight);
				}
			}
		});
	}

	function updatePadding() {
		// Calculate top padding (sum of heights above visible area)
		let topPadding = 0;
		for (let i = 0; i < start; i++) {
			topPadding += getItemHeight(i);
		}

		// Calculate bottom padding (estimated heights below visible area)
		let bottomPadding = 0;
		for (let i = end; i < data.length; i++) {
			bottomPadding += getItemHeight(i);
		}

		top = topPadding;
		bottom = bottomPadding;
	}

	async function scrollToItem(index: number) {
		if (!viewportElement || index < 0 || index >= data.length) return;

		let targetScrollTop = 0;
		for (let i = 0; i < index; i++) {
			targetScrollTop += getItemHeight(i);
		}

		viewportElement.scrollTo({
			top: targetScrollTop,
			behavior: 'smooth'
		});
	}
</script>

<Render
	class="bg-surface1 flex h-[100svh] max-h-[100svh] w-full overflow-hidden dark:bg-black"
	as={as ?? 'div'}
	{...restProps}
	{@attach (node: HTMLElement) => {
		rootElement = node;
	}}
>
	<div
		bind:this={viewportElement}
		bind:offsetHeight={viewportHeight}
		class={twMerge(
			'virtual-page-viewport relative flex h-full max-h-full w-full flex-1 flex-col overflow-y-auto',
			klass
		)}
		onscroll={handleScroll}
	>
		{@render children?.({ items: visibleItems })}
	</div>
</Render>

<style>
	.virtual-page-viewport {
		-webkit-overflow-scrolling: touch;
	}
</style>

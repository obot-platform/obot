<script lang="ts" generics="T">
	import { tick } from 'svelte';
	import { throttle } from 'es-toolkit';
	import { twMerge } from 'tailwind-merge';

	type Props = {
		class?: string;
		data?: T[];
		columns?: number;
		rowHeight?: number;
		overscan?: number;
		children: import('svelte').Snippet<[{ item: T; index: number }]>;
	};

	let {
		class: klass = '',
		data = [],
		columns = 2,
		rowHeight = 280,
		overscan = 2,
		children
	}: Props = $props();

	const rows = $derived.by(() => {
		const out: T[][] = [];
		for (let i = 0; i < data.length; i += columns) {
			out.push(data.slice(i, i + columns));
		}
		return out;
	});

	let rootElement: HTMLDivElement | undefined = $state();
	let scrollParent: HTMLElement | null = $state(null);
	let scrollTop = $state(0);
	let viewportHeight = $state(400);
	let containerTop = $state(0);

	const totalHeight = $derived(rows.length * rowHeight);

	function getScrollParent(el: HTMLElement | null): HTMLElement | null {
		if (!el) return null;
		let parent = el.parentElement;
		while (parent) {
			const style = getComputedStyle(parent);
			const overflowY = style.overflowY;
			if (
				(overflowY === 'auto' || overflowY === 'scroll' || overflowY === 'overlay') &&
				parent.scrollHeight > parent.clientHeight
			) {
				return parent;
			}
			parent = parent.parentElement;
		}
		return null;
	}

	function updateVisibility() {
		if (!rootElement || !scrollParent) return;
		scrollTop = scrollParent.scrollTop;
		viewportHeight = scrollParent.clientHeight;
		const rootRect = rootElement.getBoundingClientRect();
		const parentRect = scrollParent.getBoundingClientRect();
		containerTop = scrollParent.scrollTop + (rootRect.top - parentRect.top);
	}

	const visibleStartRow = $derived.by(() => {
		const startPixel = Math.max(0, scrollTop - containerTop - overscan * rowHeight);
		return Math.max(0, Math.floor(startPixel / rowHeight));
	});
	const visibleEndRow = $derived.by(() => {
		const endPixel = scrollTop + viewportHeight - containerTop + overscan * rowHeight;
		return Math.min(rows.length, Math.ceil(endPixel / rowHeight));
	});

	const topPadding = $derived(visibleStartRow * rowHeight);
	const bottomPadding = $derived(Math.max(0, (rows.length - visibleEndRow) * rowHeight));

	const visibleRowsSlice = $derived(
		rows.slice(visibleStartRow, visibleEndRow).map((cells, i) => ({
			rowIndex: visibleStartRow + i,
			cells
		}))
	);

	const handleScroll = throttle(updateVisibility, 1000 / 60);

	$effect(() => {
		if (!rootElement) return;
		scrollParent = getScrollParent(rootElement);
		if (!scrollParent) return;
		updateVisibility();
		scrollParent.addEventListener('scroll', handleScroll, { passive: true });
		const ro = new ResizeObserver(() => {
			tick().then(updateVisibility);
		});
		ro.observe(scrollParent);
		tick().then(updateVisibility);
		return () => {
			scrollParent?.removeEventListener('scroll', handleScroll);
			ro.disconnect();
		};
	});
</script>

<div
	bind:this={rootElement}
	class={twMerge(klass)}
	role="list"
	style="min-height: {totalHeight}px;"
>
	<div style="height: {totalHeight}px; position: relative;">
		<div style="height: {topPadding}px;" aria-hidden="true"></div>
		{#each visibleRowsSlice as { rowIndex, cells } (rowIndex)}
			<div
				class="grid grid-cols-1 gap-4 md:grid-cols-2"
				style="height: {rowHeight}px; min-height: {rowHeight}px;"
				role="listitem"
			>
				{#each cells as cell, colIndex (rowIndex * columns + colIndex)}
					{@render children?.({ item: cell, index: rowIndex * columns + colIndex })}
				{/each}
			</div>
		{/each}
		<div style="height: {bottomPadding}px;" aria-hidden="true"></div>
	</div>
</div>

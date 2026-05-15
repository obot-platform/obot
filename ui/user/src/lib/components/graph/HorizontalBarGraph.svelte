<script lang="ts" generics="T extends object">
	import HorizontalBarRow from './HorizontalBarRow.svelte';
	import { autoUpdate, computePosition, flip, offset } from '@floating-ui/dom';
	import { scaleBand, scaleLinear, select } from 'd3';
	import { axisBottom, axisLeft } from 'd3';
	import type { Snippet } from 'svelte';
	import { fade } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	/** Tooltip payload for a single bar (passed to tooltipContent snippet). */
	type TooltipItem = {
		label: string;
		value: number;
		row: T;
	};

	interface Props<T> {
		data: T[];
		labelKey: Extract<keyof T, string>;
		valueKey: Extract<keyof T, string>;
		tooltipContent?: Snippet<[TooltipItem]>;
		class?: string;
		formatValue?: (value: number) => string;
		formatLabel?: (label: T[keyof T]) => string;
	}

	let {
		data,
		labelKey,
		valueKey,
		tooltipContent,
		class: klass = '',
		formatValue = (v) => String(v),
		formatLabel = (d) => String(d)
	}: Props<T> = $props();

	let highlightedRectElement = $state<SVGGraphicsElement>();
	let currentItem = $state<TooltipItem>();
	let clientWidth = $state(0);
	let clientHeight = $state(0);

	const paddingLeft = 2;
	const paddingRight = 8;
	const paddingTop = 8;
	const paddingBottom = 24;

	const innerWidth = $derived(clientWidth - paddingLeft - paddingRight);
	const innerHeight = $derived(clientHeight - paddingTop - paddingBottom);

	const valueDomain = $derived([
		0,
		Math.max(
			1,
			...data.map((d) => {
				const n = Number(d[valueKey]);
				return Number.isFinite(n) ? n : 0;
			})
		)
	] as [number, number]);

	const xScale = $derived(scaleLinear(valueDomain, [0, innerWidth]));
	/** One band per row (by index) so duplicate labels get separate bars */
	const rowIndices = $derived(data.map((_, i) => i));
	const yScale = $derived(
		scaleBand<number>()
			.domain(rowIndices)
			.range([0, innerHeight])
			.paddingInner(0.1)
			.paddingOuter(0.1)
	);

	const barColor = '#4575b4';

	const LABEL_INSET = 8;
	const LABEL_MIN_GAP = 16;

	function estimateTextWidth(text: string): number {
		return text.length * 7;
	}

	const BAR_RADIUS = 2;

	function tooltip(reference: Element, floating: HTMLElement) {
		const compute = async () => {
			const position = await computePosition(reference, floating, {
				placement: 'top',
				middleware: [
					offset(8),
					flip({
						padding: { top: 0, right: 40, left: 40, bottom: 0 },
						boundary: document.documentElement,
						fallbackPlacements: ['top', 'top-end', 'top-start', 'left-start', 'right-start']
					})
				]
			});
			floating.style.transform = `translate(${position.x}px, ${position.y}px)`;
		};
		return autoUpdate(reference, floating, compute, {
			animationFrame: true,
			ancestorScroll: true,
			ancestorResize: true
		});
	}
</script>

<div class={twMerge('group relative flex h-full w-full flex-col', klass)}>
	<div bind:clientWidth bind:clientHeight class="min-h-0 min-w-0 flex-1">
		{#if highlightedRectElement && currentItem}
			<div
				class="tooltip bg-base-100 dark:bg-base-300 pointer-events-none fixed top-0 left-0 z-50 flex flex-col shadow-md"
				{@attach (node) => tooltip(highlightedRectElement!, node)}
				in:fade={{ duration: 100, delay: 10 }}
				out:fade={{ duration: 100 }}
			>
				{#if tooltipContent}
					{@render tooltipContent(currentItem)}
				{:else}
					<div class="flex flex-col gap-0 text-xs">
						<div class="text-sm">{currentItem?.label}</div>
					</div>
					<div class="text-lg font-semibold">
						{currentItem != null ? formatValue(currentItem.value) : ''}
					</div>
				{/if}
			</div>
		{/if}

		<svg width={clientWidth} height={clientHeight} viewBox="0 0 {clientWidth} {clientHeight}">
			<g transform="translate({paddingLeft}, {paddingTop})">
				<g
					class="x-axis text-base-content/10"
					transform="translate(0, {innerHeight})"
					{@attach (node: SVGGElement) => {
						select(node)
							.transition()
							.duration(100)
							.call(
								axisBottom(xScale)
									.tickSizeOuter(0)
									.ticks(5)
									.tickFormat((d) => formatValue(Number(d)))
							);
						select(node).selectAll('.tick line').attr('y1', -innerHeight);
					}}
				></g>

				<g
					class="y-axis text-base-content/10"
					{@attach (node: SVGGElement) => {
						select(node)
							.transition()
							.duration(100)
							.call(axisLeft(yScale).tickSizeOuter(0).tickSize(-innerWidth));
						select(node).selectAll('.tick line').attr('stroke-opacity', 0.1);
						select(node).selectAll('.tick text').remove();
						select(node).select('.domain').attr('opacity', 0);
					}}
				></g>

				<g class="data">
					{#each data as row, i (i)}
						{@const label = formatLabel(row[labelKey])}
						{@const raw = Number(row[valueKey])}
						{@const value = Number.isFinite(raw) ? raw : 0}
						{@const y = yScale(i) ?? 0}
						{@const height = yScale.bandwidth()}
						{@const width = Math.max(0, xScale(value) - xScale(0))}
						{@const textWidth = estimateTextWidth(label)}
						{@const fitsInside = width > textWidth + LABEL_MIN_GAP}
						{@const textX = fitsInside ? LABEL_INSET : width + LABEL_INSET}
						{@const textAnchor = 'start'}
						{@const barY = y + height / 2}
						<HorizontalBarRow
							targetWidth={width}
							{y}
							{height}
							fill={barColor}
							rx={BAR_RADIUS}
							class="cursor-pointer"
							onpointerenter={(e) => {
								highlightedRectElement = e.currentTarget as SVGGraphicsElement;
								currentItem = { label, value, row };
								select(e.currentTarget as SVGGraphicsElement)
									.attr('stroke', 'currentColor')
									.attr('stroke-width', 2);
							}}
							onpointerleave={(e) => {
								if (e.currentTarget === highlightedRectElement) {
									highlightedRectElement = undefined;
								}
								select(e.currentTarget as SVGGraphicsElement).attr('stroke-width', 0);
							}}
						/>
						<text
							x={textX}
							y={barY}
							text-anchor={textAnchor}
							dominant-baseline="central"
							fill={fitsInside ? 'white' : 'currentColor'}
							font-size="12"
							font-weight="500"
							class="text-muted-content pointer-events-none"
						>
							{label}
						</text>
					{/each}
				</g>
			</g>
		</svg>
	</div>
</div>

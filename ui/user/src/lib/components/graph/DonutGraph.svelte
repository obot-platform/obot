<script lang="ts">
	import { darkMode } from '$lib/stores';
	import { autoUpdate, computePosition, flip, offset } from '@floating-ui/dom';
	import { arc, pie, type PieArcDatum } from 'd3';
	import type { Snippet } from 'svelte';
	import { cubicInOut, cubicOut } from 'svelte/easing';
	import { Tween } from 'svelte/motion';
	import { fade } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	export type DonutDatum = {
		label: string;
		value: number;
		color?: string;
		/**
		 * Same groupKey = no radial divider between adjacent arcs (e.g. status shades within one entry type).
		 * Omit for default: divider after every arc.
		 */
		groupKey?: string;
	};

	export type DonutLegendItem = {
		label: string;
		color?: string;
	};

	/** Tooltip payload for a slice (passed to `tooltipContent` snippet). */
	export type DonutTooltipItem = {
		label: string;
		value: number;
		/** Share of total in [0, 100]. */
		percentOfTotal: number;
	};

	interface Props {
		data: DonutDatum[];
		class?: string;
		/** Inner radius as a fraction of the outer radius (0 = pie, 1 = invisible). */
		donutRatio?: number;
		formatValue?: (value: number) => string;
		tooltipContent?: Snippet<[DonutTooltipItem]>;
		/** When set, the footer legend shows these entries instead of one row per slice. */
		legend?: DonutLegendItem[];
		/** Hides the footer legend entirely. */
		hideLegend?: boolean;
	}

	let {
		data,
		class: klass = '',
		donutRatio = 0.58,
		formatValue = (v) => String(v),
		tooltipContent,
		legend,
		hideLegend = false
	}: Props = $props();

	const defaultPalette = [
		'#4575b4',
		'#fee090',
		'#fdae61',
		'#f46d43',
		'#d73027',
		'#e0f3f8',
		'#abd9e9',
		'#74add1'
	];

	const grayColor = $derived(darkMode.isDark ? '#999999' : '#cccccc');

	function colorAt(i: number) {
		return i < defaultPalette.length ? (defaultPalette[i] ?? grayColor) : grayColor;
	}

	const pieGenerator = pie<DonutDatum>()
		.sort(null)
		.value((d) => Math.max(0, Number(d.value) || 0));

	let clientWidth = $state(0);
	let clientHeight = $state(0);

	const size = $derived(Math.min(Math.max(clientWidth, 1), Math.max(clientHeight, 1), 320));
	const outerRadius = $derived(Math.max(0, size / 2 - 2));
	const innerRadius = $derived(outerRadius * donutRatio);

	/** Slightly enlarges the hovered wedge radially (outer radius multiplier at full hover). */
	const HOVER_OUTER_RADIUS_SCALE = 1.025;

	/**
	 * ViewBox half-extent in user units so the hovered wedge (max radius
	 * `outerRadius * HOVER_OUTER_RADIUS_SCALE`) and strokes fit without clipping.
	 * Wider viewBox than `size/2` slightly scales the chart down within the same pixel `size`.
	 */
	const VIEWBOX_EDGE_PAD = 4;
	const viewBoxHalf = $derived(
		outerRadius > 0 ? outerRadius * HOVER_OUTER_RADIUS_SCALE + VIEWBOX_EDGE_PAD : 1
	);

	const total = $derived(data.reduce((sum, d) => sum + Math.max(0, Number(d.value) || 0), 0));

	const useGroupSeparators = $derived(
		data.some((d) => d.groupKey !== undefined && d.groupKey !== '')
	);

	const dataKey = $derived(
		data.map((d) => `${d.label}:${d.value}:${d.color ?? ''}:${d.groupKey ?? ''}`).join('|')
	);

	const progress = new Tween(0, { duration: 550, easing: cubicOut });

	/** Animates hover expansion/collapse (interpolates toward `HOVER_OUTER_RADIUS_SCALE`). */
	const HOVER_EXPAND_MS = 200;
	const hoverExpand = new Tween(0, { duration: HOVER_EXPAND_MS, easing: cubicOut });

	/** Crossfade when moving hover from one slice to another (shrink old / grow new). */
	const HANDOFF_MS = 200;
	const handoffProgress = new Tween(0, { duration: HANDOFF_MS, easing: cubicInOut });

	let hoveredSliceIndex = $state<number | undefined>();
	let handoff = $state<{ from: number; to: number } | null>(null);
	let handoffGen = 0;
	let hoverClearTimeout: ReturnType<typeof setTimeout> | undefined;
	/** Invalidates a pending collapse so `.then` does not clear hover after re-entry. */
	let pendingHoverCollapse: symbol | undefined;

	function invalidateHandoffTween() {
		handoffGen += 1;
		void handoffProgress.set(0, { duration: 0 });
	}

	function cancelHoverSliceClear() {
		if (hoverClearTimeout !== undefined) {
			clearTimeout(hoverClearTimeout);
			hoverClearTimeout = undefined;
		}
		pendingHoverCollapse = undefined;
	}

	function startHandoff(from: number, to: number) {
		handoffGen += 1;
		const id = handoffGen;
		handoff = { from, to };
		void handoffProgress.set(0, { duration: 0 }).then(
			() =>
				void handoffProgress.set(1).then(() => {
					if (handoffGen === id) {
						hoveredSliceIndex = to;
						handoff = null;
					}
				})
		);
	}

	function scheduleHoverSliceClear() {
		cancelHoverSliceClear();
		handoff = null;
		invalidateHandoffTween();
		const token = Symbol();
		pendingHoverCollapse = token;
		hoverClearTimeout = setTimeout(() => {
			hoverClearTimeout = undefined;
			void hoverExpand.set(0).then(() => {
				if (pendingHoverCollapse === token) {
					hoveredSliceIndex = undefined;
					pendingHoverCollapse = undefined;
				}
			});
		}, 0);
	}

	function scheduleDonutEntryAnimation(_seriesKey: string) {
		void progress.set(0, { duration: 0 }).then(() => void progress.set(1));
	}

	$effect(() => {
		if (total <= 0) {
			void progress.set(0, { duration: 0 });
			void hoverExpand.set(0, { duration: 0 });
			hoveredSliceIndex = undefined;
			handoff = null;
			invalidateHandoffTween();
			cancelHoverSliceClear();
			return;
		}
		scheduleDonutEntryAnimation(dataKey);
	});

	$effect(() => {
		return () => cancelHoverSliceClear();
	});

	let highlightedArcElement = $state<SVGGraphicsElement>();
	let currentItem = $state<DonutTooltipItem>();

	function tooltip(reference: Element, floating: HTMLElement) {
		const compute = async () => {
			const position = await computePosition(reference, floating, {
				placement: 'top',
				middleware: [
					offset(8),
					flip({
						padding: {
							top: 0,
							right: 40,
							left: 40,
							bottom: 0
						},
						boundary: document.documentElement,
						fallbackPlacements: ['top', 'top-end', 'top-start', 'left-start', 'right-start']
					})
				]
			});

			const { x, y } = position;

			floating.style.transform = `translate(${x}px, ${y}px)`;
		};

		return autoUpdate(reference, floating, compute, {
			animationFrame: true,
			ancestorScroll: true,
			ancestorResize: true
		});
	}

	const TOOLTIP_LABEL_SEP = ' · ';

	function tooltipLabelParts(label: string): { main: string; detail?: string } {
		const i = label.indexOf(TOOLTIP_LABEL_SEP);
		if (i === -1) return { main: label };
		return {
			main: label.slice(0, i),
			detail: label.slice(i + TOOLTIP_LABEL_SEP.length)
		};
	}

	function sliceGroupKey(d: PieArcDatum<DonutDatum>, i: number) {
		const g = d.data.groupKey;
		return g !== undefined && g !== '' ? g : `__slice_${i}`;
	}

	/** Radial line at angle a (d3 pie convention): inner → outer, center (0,0). */
	function radialSeparatorPath(a: number, outerEnd: number) {
		const x1 = innerRadius * Math.sin(a);
		const y1 = -innerRadius * Math.cos(a);
		const x2 = outerEnd * Math.sin(a);
		const y2 = -outerEnd * Math.cos(a);
		return `M${x1},${y1}L${x2},${y2}`;
	}

	type SliceBundle = {
		arcs: { path: string; color: string; label: string; value: number }[];
		separators: string[];
	};

	function computeSlices(
		p: number,
		hoverIndex: number | undefined,
		hoverT: number,
		handoffPair: { from: number; to: number } | null,
		handoffU: number
	): SliceBundle {
		if (outerRadius <= 0 || total <= 0) return { arcs: [], separators: [] };
		const snapshot = $state.snapshot(data);
		const layout = pieGenerator(snapshot);
		const n = layout.length;

		const dUnit = HOVER_OUTER_RADIUS_SCALE - 1;

		function sliceOuterAt(i: number): number {
			if (handoffPair !== null && hoverT > 0) {
				const d = dUnit * hoverT;
				const u = handoffU;
				const { from, to } = handoffPair;
				if (i === from) return outerRadius * (1 + d * (1 - 2 * u));
				if (i === to) return outerRadius * (1 + d * (2 * u - 1));
				return outerRadius * (1 - d);
			}

			const hoverDelta = hoverIndex !== undefined && hoverT > 0 ? dUnit * hoverT : 0;
			if (hoverDelta <= 0 || hoverIndex === undefined) return outerRadius;
			if (hoverIndex === i) return outerRadius * (1 + hoverDelta);
			return outerRadius * (1 - hoverDelta);
		}

		const arcs = layout.map((d, i) => {
			const sliceOuter = sliceOuterAt(i);
			const arcGen = arc<PieArcDatum<DonutDatum>>()
				.innerRadius(innerRadius)
				.outerRadius(sliceOuter)
				.cornerRadius(useGroupSeparators ? 0 : 0.5);
			const scaled: PieArcDatum<DonutDatum> = {
				...d,
				endAngle: d.startAngle + (d.endAngle - d.startAngle) * p
			};
			return {
				path: arcGen(scaled) ?? '',
				color: d.data.color ?? colorAt(i),
				label: d.data.label,
				value: d.data.value
			};
		});

		const separators: string[] = [];
		if (useGroupSeparators && n >= 2) {
			for (let i = 0; i < n; i++) {
				const a = layout[i]!;
				const b = layout[(i + 1) % n]!;
				if (sliceGroupKey(a, i) !== sliceGroupKey(b, (i + 1) % n)) {
					const scaledEnd = a.startAngle + (a.endAngle - a.startAngle) * p;
					const outerEnd = Math.max(sliceOuterAt(i), sliceOuterAt((i + 1) % n));
					separators.push(radialSeparatorPath(scaledEnd, outerEnd));
				}
			}
		}

		return { arcs, separators };
	}

	const donut = $derived(
		computeSlices(
			progress.current,
			hoveredSliceIndex,
			hoverExpand.current,
			handoff,
			handoffProgress.current
		)
	);
</script>

<div class={twMerge('group relative flex min-h-0 min-w-0 flex-col gap-3', klass)}>
	<div
		bind:clientWidth
		bind:clientHeight
		class="relative flex min-h-[160px] w-full flex-1 items-center justify-center"
	>
		{#if highlightedArcElement && currentItem}
			<div
				class="tooltip bg-background dark:bg-surface2 pointer-events-none fixed top-0 left-0 z-50 flex flex-col shadow-md min-w-32"
				{@attach (node) => tooltip(highlightedArcElement!, node)}
				in:fade={{ duration: 100, delay: 10 }}
				out:fade={{ duration: 100 }}
			>
				{#if tooltipContent}
					{@render tooltipContent(currentItem)}
				{:else}
					{@const parts = tooltipLabelParts(currentItem.label)}
					<div class="flex flex-col gap-0 text-xs">
						<div class="text-on-background text-sm">
							{#if parts.detail !== undefined}
								<span class="font-semibold">{parts.main}</span>
								<span>{TOOLTIP_LABEL_SEP}{parts.detail}</span>
							{:else}
								<span class="font-semibold">{parts.main}</span>
							{/if}
						</div>
						<div class="border-on-surface1 mb-2 border-b pb-2">
							{currentItem.percentOfTotal.toFixed(1)}% of total
						</div>
					</div>
					<div class="text-on-background text-2xl font-bold">{formatValue(currentItem.value)}</div>
				{/if}
			</div>
		{/if}

		{#if total <= 0}
			<p class="text-on-surface1 font-light text-sm">No data</p>
		{:else}
			<svg
				class="max-h-full max-w-full overflow-visible"
				width={size}
				height={size}
				viewBox="{-viewBoxHalf} {-viewBoxHalf} {viewBoxHalf * 2} {viewBoxHalf * 2}"
				role="img"
				aria-label="Donut chart"
			>
				<g>
					{#each donut.arcs as slice, i (i)}
						<path
							role="graphics-symbol"
							aria-label="{slice.label}, {formatValue(Math.max(0, Number(slice.value) || 0))}"
							d={slice.path}
							fill={slice.color}
							class={twMerge(
								'cursor-pointer stroke-1',
								useGroupSeparators ? '' : 'stroke-background dark:stroke-surface1'
							)}
							onpointerenter={(e) => {
								cancelHoverSliceClear();
								if (handoff !== null) {
									hoveredSliceIndex = handoff.to;
									handoff = null;
									invalidateHandoffTween();
								}
								const prev = hoveredSliceIndex;
								if (prev !== undefined && prev !== i && hoverExpand.current > 0.001) {
									startHandoff(prev, i);
								} else {
									hoveredSliceIndex = i;
									void hoverExpand.set(1);
								}
								const el = e.currentTarget as SVGGraphicsElement;
								highlightedArcElement = el;
								const raw = Math.max(0, Number(slice.value) || 0);
								currentItem = {
									label: slice.label,
									value: raw,
									percentOfTotal: total > 0 ? (raw / total) * 100 : 0
								};
							}}
							onpointerleave={(e) => {
								scheduleHoverSliceClear();
								if (e.currentTarget === highlightedArcElement) {
									highlightedArcElement = undefined;
									currentItem = undefined;
								}
							}}
						/>
					{/each}
					{#each donut.separators as sepPath, sepIdx (sepIdx)}
						<path
							d={sepPath}
							fill="none"
							class="stroke-background dark:stroke-surface1 stroke-1 pointer-events-none"
							aria-hidden="true"
						/>
					{/each}
				</g>
			</svg>
		{/if}
	</div>

	{#if total > 0 && data.length > 0 && !hideLegend}
		<ul class="flex flex-wrap gap-x-4 gap-y-1 text-xs justify-center">
			{#if legend}
				{#each legend as row, i (row.label)}
					<li class="flex items-center gap-1.5">
						<span
							class="inline-block size-2.5 shrink-0 rounded-sm"
							style:background-color={row.color ?? colorAt(i)}
							aria-hidden="true"
						></span>
						<span class="text-foreground">{row.label}</span>
					</li>
				{/each}
			{:else}
				{#each data as row, i (i)}
					<li class="flex items-center gap-1.5">
						<span
							class="inline-block size-2.5 shrink-0 rounded-sm"
							style:background-color={row.color ?? colorAt(i)}
							aria-hidden="true"
						></span>
						<span class="text-foreground">{row.label}</span>
					</li>
				{/each}
			{/if}
		</ul>
	{/if}
</div>

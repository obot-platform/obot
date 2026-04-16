<script lang="ts">
	import { darkMode } from '$lib/stores';
	import { autoUpdate, computePosition, flip, offset } from '@floating-ui/dom';
	import { arc, pie, select, type PieArcDatum } from 'd3';
	import type { Snippet } from 'svelte';
	import { cubicOut } from 'svelte/easing';
	import { Tween } from 'svelte/motion';
	import { fade } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	export type DonutDatum = {
		label: string;
		value: number;
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
	}

	let {
		data,
		class: klass = '',
		donutRatio = 0.58,
		formatValue = (v) => String(v),
		tooltipContent
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

	const total = $derived(data.reduce((sum, d) => sum + Math.max(0, Number(d.value) || 0), 0));

	const dataKey = $derived(data.map((d) => `${d.label}:${d.value}`).join('|'));

	const progress = new Tween(0, { duration: 550, easing: cubicOut });

	function scheduleDonutEntryAnimation(_seriesKey: string) {
		void progress.set(0, { duration: 0 }).then(() => void progress.set(1));
	}

	$effect(() => {
		if (total <= 0) {
			void progress.set(0, { duration: 0 });
			return;
		}
		scheduleDonutEntryAnimation(dataKey);
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

	function computeSlices(p: number) {
		if (outerRadius <= 0 || total <= 0) return [];
		const snapshot = $state.snapshot(data);
		const arcGen = arc<PieArcDatum<DonutDatum>>()
			.innerRadius(innerRadius)
			.outerRadius(outerRadius)
			.cornerRadius(0.5);
		return pieGenerator(snapshot).map((d, i) => {
			const scaled: PieArcDatum<DonutDatum> = {
				...d,
				endAngle: d.startAngle + (d.endAngle - d.startAngle) * p
			};
			return {
				path: arcGen(scaled) ?? '',
				color: colorAt(i),
				label: d.data.label,
				value: d.data.value
			};
		});
	}
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
					<div class="flex flex-col gap-0 text-xs">
						<div class="text-on-background text-sm">{currentItem.label}</div>
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
				class="max-h-full max-w-full"
				width={size}
				height={size}
				viewBox="{-size / 2} {-size / 2} {size} {size}"
				role="img"
				aria-label="Donut chart"
			>
				<g>
					{#each computeSlices(progress.current) as slice, i (i)}
						<path
							role="graphics-symbol"
							aria-label="{slice.label}, {formatValue(Math.max(0, Number(slice.value) || 0))}"
							d={slice.path}
							fill={slice.color}
							class="cursor-pointer stroke-background stroke-1"
							onpointerenter={(e) => {
								const el = e.currentTarget as SVGGraphicsElement;
								highlightedArcElement = el;
								const raw = Math.max(0, Number(slice.value) || 0);
								currentItem = {
									label: slice.label,
									value: raw,
									percentOfTotal: total > 0 ? (raw / total) * 100 : 0
								};
								select(el).attr('stroke', 'currentColor').attr('stroke-width', 2);
							}}
							onpointerleave={(e) => {
								if (e.currentTarget === highlightedArcElement) {
									highlightedArcElement = undefined;
									currentItem = undefined;
								}
								select(e.currentTarget as SVGGraphicsElement)
									.attr('stroke-width', 0)
									.attr('stroke', null);
							}}
						/>
					{/each}
				</g>
			</svg>
		{/if}
	</div>

	{#if total > 0 && data.length > 0}
		<ul class="flex flex-wrap gap-x-4 gap-y-1 text-xs justify-center">
			{#each data as row, i (i)}
				<li class="flex items-center gap-1.5">
					<span
						class="inline-block size-2.5 shrink-0 rounded-sm"
						style:background-color={colorAt(i)}
						aria-hidden="true"
					></span>
					<span class="text-foreground">{row.label}</span>
				</li>
			{/each}
		</ul>
	{/if}
</div>

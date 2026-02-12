<script lang="ts">
	import { darkMode } from '$lib/stores';
	import { BarChart, Bars, Legend } from 'layerchart';
	import type { ComponentProps } from 'svelte';
	import { tick } from 'svelte';
	import { SvelteSet } from 'svelte/reactivity';
	import { cubicInOut } from 'svelte/easing';
	import { parseColorToHsl } from '$lib/colors';

	// fix for axis lines being rendered in front of bars
	function moveMarksAboveAxis(node: SVGGElement) {
		let parent: SVGElement | null = null;
		const run = () => {
			parent = node.parentElement as SVGElement | null;
			if (parent) parent.appendChild(node);
		};
		tick().then(run);
		return {
			destroy() {}
		};
	}

	type BarChartProps = ComponentProps<typeof BarChart>;
	type Props = {
		data: BarChartProps['data'];
		series?: BarChartProps['series'];
		height?: number;
		tweened?: boolean;
	};

	let { data, series, height = 384, tweened }: Props = $props();

	function hslToCss(h: number, s: number, l: number): string {
		return `hsl(${h.toFixed(1)} ${s.toFixed(1)}% ${l.toFixed(1)}%)`;
	}

	function generateChartPalette(
		primaryHsl: { h: number; s: number; l: number },
		n: number
	): string[] {
		if (n <= 0) return [];
		if (n === 1) return [hslToCss(primaryHsl.h, primaryHsl.s, primaryHsl.l)];
		const { h, s, l } = primaryHsl;
		const minL = 30;
		const maxL = 88;
		const step = 14;
		const colors = [hslToCss(h, s, l)];
		for (let i = 1; i < n; i++) {
			const stepIndex = Math.ceil(i / 2);
			const delta = i % 2 === 1 ? stepIndex * step : -stepIndex * step;
			const lNew = Math.max(minL, Math.min(maxL, l + delta));
			const sNew = Math.max(50, Math.min(100, s - stepIndex * 2));
			colors.push(hslToCss(h, sNew, lNew));
		}
		return colors;
	}

	let primaryColorCss = $state<string | null>(null);
	$effect(() => {
		void darkMode.isDark;
		const el = typeof document !== 'undefined' ? document.documentElement : null;
		if (!el) return;
		primaryColorCss = getComputedStyle(el).getPropertyValue('--color-primary').trim() || null;
	});

	function getChartColors(n: number): string[] {
		const primary = primaryColorCss ? parseColorToHsl(primaryColorCss) : null;
		if (!primary) {
			const fallback = parseColorToHsl('#4f7ef3');
			return generateChartPalette(fallback!, Math.max(1, n));
		}
		return generateChartPalette(primary, Math.max(1, n));
	}

	const effectiveSeries = $derived.by(() => {
		if (series != null && series.length > 0) return series;
		const keys = new SvelteSet<string>();
		for (const row of data as Record<string, unknown>[]) {
			for (const key of Object.keys(row)) {
				if (key !== 'bucket') keys.add(key);
			}
		}
		const sortedKeys = [...keys].sort();
		const palette = getChartColors(sortedKeys.length);
		return sortedKeys.map((key, i) => ({
			key,
			color: palette[i] ?? palette[0]
		}));
	});

	const chartTextClass = $derived(darkMode.isDark ? 'fill-white' : 'fill-black');
	const chartHighlightFill = $derived(
		darkMode.isDark ? 'rgba(0, 0, 0, 0.25)' : 'rgba(0, 0, 0, 0.05)'
	);
	const chartTooltipRootClass = $derived(
		darkMode.isDark
			? '!bg-surface2/90 rounded-sm p-2 !text-white [&_.label]:!text-white/75 [&_.value]:!text-left [&_div:has(>.label>.color)>.value]:!pl-4 border border-surface3'
			: '!bg-white/90 shadow-sm rounded-sm p-2 !text-black [&_.label]:!text-black/75 [&_.value]:!text-left [&_div:has(>.label>.color)>.value]:!pl-4'
	);
	/** Force tooltip list to a single column so all legend items stack vertically (override library's 2-column grid). */
	const chartTooltipListClass = $derived('!grid !grid-cols-1 !gap-y-0.5 !gap-x-0');
	const chartAxisRuleClass = 'stroke-[var(--color-on-background)]/30';
	const chartGridClass = 'stroke-[var(--color-on-background)]/10';

	/** Ordinal scale for the legend: domain = keys, range = colors, callable as scale(key) => color. */
	const legendScale = $derived.by(() => {
		const keys = effectiveSeries.map((s) => s.key);
		const colors = effectiveSeries.map((s) => s.color);
		const map = new Map(keys.map((k, i) => [k, colors[i] ?? colors[0]]));
		const scale = (key: string): string => map.get(key) ?? colors[0] ?? '';
		scale.domain = () => keys;
		scale.range = () => colors;
		return scale;
	});
</script>

<div class="stacked-graph flex min-h-0 min-w-0 flex-col" style="min-height: {height}px;">
	<div class="min-w-0 shrink-0" style="height: {height}px;">
		<BarChart
			{data}
			x="bucket"
			series={effectiveSeries}
			seriesLayout="stack"
			legend={false}
			props={{
				yAxis: {
					format: 'metric',
					rule: true,
					grid: { class: chartGridClass },
					classes: { tickLabel: chartTextClass, rule: chartAxisRuleClass }
				},
				tooltip: {
					root: { class: chartTooltipRootClass },
					header: { format: 'none', class: chartTextClass },
					list: { class: `${chartTextClass} ${chartTooltipListClass}` },
					item: { class: chartTextClass }
				},
				bars: {
					strokeWidth: 0,
					radius: 0,
					tweened: tweened ? { easing: cubicInOut } : false
				},
				highlight: {
					area: { fill: chartHighlightFill }
				}
			}}
		>
			<g
				class="stacked-graph-marks"
				use:moveMarksAboveAxis
				slot="marks"
				let:getBarsProps
				let:visibleSeries
			>
				{#each [...visibleSeries].reverse() as s, revIdx (s.key)}
					{@const i = visibleSeries.length - 1 - revIdx}
					<Bars
						{...getBarsProps(s, i)}
						tweened={tweened ? { duration: 150, easing: cubicInOut, delay: i * 30 } : false}
					/>
				{/each}
			</g>
		</BarChart>
	</div>
	<div class="mt-3 min-w-0 flex-shrink-0">
		<Legend
			scale={legendScale}
			tickFormat={(key) => effectiveSeries.find((s) => s.key === key)?.key ?? key}
			variant="swatches"
			classes={{
				root: 'max-w-full min-w-0',
				swatches: 'flex-wrap gap-y-2 justify-center items-center'
			}}
		/>
	</div>
</div>

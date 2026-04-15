<script lang="ts">
	import { darkMode } from '$lib/stores';
	import { arc, pie, type PieArcDatum } from 'd3';
	import { twMerge } from 'tailwind-merge';

	export type DonutDatum = {
		label: string;
		value: number;
	};

	interface Props {
		data: DonutDatum[];
		class?: string;
		/** Inner radius as a fraction of the outer radius (0 = pie, 1 = invisible). */
		donutRatio?: number;
		formatValue?: (value: number) => string;
	}

	let {
		data,
		class: klass = '',
		donutRatio = 0.58,
		formatValue = (v) => String(v)
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

	const slices = $derived.by(() => {
		if (outerRadius <= 0 || total <= 0) return [];
		const snapshot = $state.snapshot(data);
		const arcGen = arc<PieArcDatum<DonutDatum>>()
			.innerRadius(innerRadius)
			.outerRadius(outerRadius)
			.cornerRadius(0.5);
		return pieGenerator(snapshot).map((d, i) => ({
			path: arcGen(d) ?? '',
			color: colorAt(i),
			label: d.data.label,
			value: d.data.value
		}));
	});
</script>

<div class={twMerge('flex min-h-0 min-w-0 flex-col gap-3', klass)}>
	<div
		bind:clientWidth
		bind:clientHeight
		class="relative flex min-h-[160px] w-full flex-1 items-center justify-center"
	>
		{#if total <= 0}
			<p class="text-muted-foreground text-sm">No data</p>
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
					{#each slices as slice, i (i)}
						<path d={slice.path} fill={slice.color} class="stroke-background stroke-1" />
					{/each}
				</g>
				<text
					text-anchor="middle"
					dominant-baseline="central"
					class="fill-foreground pointer-events-none text-sm font-medium tabular-nums"
				>
					{formatValue(total)}
				</text>
			</svg>
		{/if}
	</div>

	{#if total > 0 && data.length > 0}
		<ul class="flex flex-wrap gap-x-4 gap-y-1 text-xs">
			{#each data as row, i (i)}
				<li class="flex items-center gap-1.5">
					<span
						class="inline-block size-2.5 shrink-0 rounded-sm"
						style:background-color={colorAt(i)}
						aria-hidden="true"
					></span>
					<span class="text-foreground">{row.label}</span>
					<span class="text-muted-foreground tabular-nums"
						>{formatValue(Number(row.value) || 0)}</span
					>
				</li>
			{/each}
		</ul>
	{/if}
</div>

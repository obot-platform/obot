<script lang="ts" generics="T">
	import { Chart, Axis, Spline, Tooltip, Highlight, Svg } from 'layerchart';
	import { scaleTime } from 'd3-scale';
	import { formatTime } from '$lib/time';
	import { onMount } from 'svelte';

	interface LineGraphProps {
		data: T[];
		x: keyof T & string;
		y: keyof T & string;
		padding?: number;
		formatTooltipText?: (data: T) => string;
		formatXLabel?: (d: T[keyof T]) => string;
	}

	let { data, x, y, padding, formatTooltipText, formatXLabel }: LineGraphProps = $props();
	let show = $state(false);

	onMount(() => {
		setTimeout(() => {
			show = true;
		}, 300);
	});
</script>

<div class="group h-[300px] w-full">
	{#key data.length}
		<Chart
			{data}
			{x}
			xScale={scaleTime()}
			{y}
			yNice
			padding={{ left: padding, right: padding, bottom: padding, top: padding }}
			tooltip={{ mode: 'voronoi' }}
		>
			<Svg>
				<Axis placement="left" grid rule />
				<Axis placement="bottom" format={(d) => (formatXLabel ? formatXLabel(d) : d)} rule />
				{#if show}
					<Spline draw class="stroke-primary stroke-2" />
				{/if}
				<Highlight points lines />
			</Svg>

			<Tooltip.Root
				let:data
				class="dark:border-surface3 dark:bg-surface2 min-w-32 rounded-lg border border-gray-200 bg-white p-2 shadow-sm"
			>
				<Tooltip.Header class="mb-2 text-sm font-medium text-gray-900 dark:text-gray-100"
					>{formatTime(data.date)}</Tooltip.Header
				>
				<Tooltip.List class="space-y-1">
					<Tooltip.Item
						label=""
						value={formatTooltipText ? formatTooltipText(data) : data.value}
						class="text-sm text-gray-600 dark:text-gray-300"
					/>
				</Tooltip.List>
			</Tooltip.Root>
		</Chart>
	{/key}
</div>

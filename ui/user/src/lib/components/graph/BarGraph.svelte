<script lang="ts" generics="T extends object">
	import { scaleBand } from 'd3-scale';
	import { Axis, Chart, Svg, Tooltip, Highlight, RectClipPath, Bars } from 'layerchart';
	import { onMount } from 'svelte';
	import { cubicInOut } from 'svelte/easing';

	interface Props {
		data: T[];
		x: keyof T & string;
		y: keyof T & string;
		padding?: number;
		formatTooltipText?: (data: T) => string;
		formatXLabel?: (d: T[keyof T]) => string;
	}

	let { data, x, y, formatXLabel, padding, formatTooltipText }: Props = $props();
	let show = $state(false);

	onMount(() => {
		setTimeout(() => {
			show = true;
		}, 300);
	});
</script>

<div class="group h-[300px] w-full">
	<Chart
		{data}
		{x}
		xScale={scaleBand().padding(0.4)}
		{y}
		yDomain={[0, null]}
		yNice={4}
		padding={{ left: padding, bottom: padding, top: padding, right: padding }}
		tooltip={{ mode: 'band' }}
	>
		<Svg>
			<Axis placement="left" grid rule />
			<Axis placement="bottom" format={(d) => formatXLabel?.(d) ?? d} rule />
			{#if show}
				<Bars
					initialY={300 - 16 * 2 - 2 - 24}
					initialHeight={0}
					tweened={{
						y: { duration: 500, easing: cubicInOut },
						height: { duration: 500, easing: cubicInOut }
					}}
					class="fill-primary transition-colors group-hover:fill-gray-400/50 dark:group-hover:fill-gray-600/50"
				/>
			{/if}
			<Highlight bar>
				<svelte:fragment slot="area" let:area>
					<RectClipPath x={area.x} y={area.y} width={area.width} height={area.height} spring>
						<Bars class="fill-primary" />
					</RectClipPath>
				</svelte:fragment>
			</Highlight>
		</Svg>
		<Tooltip.Root let:data>
			<Tooltip.Header class="mb-2 text-sm font-medium text-gray-900 dark:text-gray-100">
				{formatXLabel?.(data[x]) ?? data[x]}
			</Tooltip.Header>
			<Tooltip.List class="space-y-1">
				<Tooltip.Item
					label=""
					value={formatTooltipText ? formatTooltipText(data) : data.value}
					class="text-sm text-gray-600 dark:text-gray-300"
				/>
			</Tooltip.List>
		</Tooltip.Root>
	</Chart>
</div>

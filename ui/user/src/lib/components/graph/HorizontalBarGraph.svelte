<script lang="ts" generics="T extends object">
	import { scaleBand, scaleLinear } from 'd3';
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

	let xDomain = $derived([0, Math.max(...data.map((d) => Number(d[y])))]);

	onMount(() => {
		setTimeout(() => {
			show = true;
		}, 300);
	});

	function estimateTextWidth(text: string): number {
		return text.length * 7;
	}
</script>

<div class="group h-[300px] w-full">
	<Chart
		{data}
		x={y}
		y={x}
		{xDomain}
		xScale={scaleLinear()}
		yScale={scaleBand().paddingInner(0.1).paddingOuter(1)}
		yDomain={data.map((d) => String(d[x]))}
		padding={{ left: padding, bottom: padding, top: padding, right: padding }}
		tooltip={{ mode: 'band' }}
		let:xScale
		let:yScale
	>
		<Svg>
			<Axis
				placement="bottom"
				grid
				rule
				ticks={() => {
					let max = Math.max(...data.map((d) => Number(d[y])));
					if (max === 0) {
						return [0];
					}
					if (max <= 1) {
						return [0, 1];
					}
					return [0, max];
				}}
			/>

			{#if show}
				<Bars
					tweened={{
						x: { duration: 500, easing: cubicInOut },
						width: { duration: 500, easing: cubicInOut }
					}}
					class="fill-primary transition-colors group-hover:fill-gray-400/50 dark:group-hover:fill-gray-600/50"
				/>
			{/if}

			{#if show && xScale && yScale}
				{#each data as item (item[x])}
					{@const barWidth = xScale(item[y]) - xScale(0)}
					{@const barY = yScale(item[x]) + yScale.bandwidth() / 2}
					{@const labelText = formatXLabel ? formatXLabel(item[x]) : String(item[x])}
					{@const textWidth = estimateTextWidth(labelText)}
					{@const fitsInside = barWidth > textWidth + 16}
					{@const textX = fitsInside ? xScale(0) + barWidth / 2 : xScale(item[y]) + 8}
					{@const textAnchor = fitsInside ? 'middle' : 'start'}
					{@const textFill = fitsInside ? 'white' : 'currentColor'}

					<text
						x={textX}
						y={barY}
						text-anchor={textAnchor}
						dominant-baseline="central"
						fill={textFill}
						font-size="12"
						font-weight="500"
						class="pointer-events-none"
					>
						{labelText}
					</text>
				{/each}
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

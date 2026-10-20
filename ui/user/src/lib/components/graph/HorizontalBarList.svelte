<script lang="ts" generics="T extends object">
	import { autoUpdate, computePosition, flip, offset } from '@floating-ui/dom';
	import type { Snippet } from 'svelte';
	import { fade } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

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

	let hoveredRow = $state<HTMLElement>();
	let currentItem = $state<TooltipItem>();

	const maxValue = $derived(
		Math.max(
			1,
			...data.map((d) => {
				const n = Number(d[valueKey]);
				return Number.isFinite(n) ? n : 0;
			})
		)
	);

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

<div class={twMerge('relative w-full @container/horizontalbarlist', klass)}>
	{#if hoveredRow && currentItem}
		<div
			class="tooltip bg-base-100 dark:bg-base-300 pointer-events-none fixed top-0 left-0 z-50 flex flex-col shadow-md"
			{@attach (node) => tooltip(hoveredRow!, node)}
			in:fade={{ duration: 100, delay: 10 }}
			out:fade={{ duration: 100 }}
		>
			{#if tooltipContent}
				{@render tooltipContent(currentItem)}
			{:else}
				<div class="flex flex-col gap-0 text-xs">
					<div class="text-sm">{currentItem.label}</div>
				</div>
				<div class="text-lg font-semibold">
					{formatValue(currentItem.value)}
				</div>
			{/if}
		</div>
	{/if}

	<div
		class="grid w-full grid-cols-[minmax(0,max-content)_minmax(0,1fr)_auto] items-center gap-x-3 gap-y-2"
	>
		{#each data as row, i (i)}
			{@const label = formatLabel(row[labelKey])}
			{@const raw = Number(row[valueKey])}
			{@const value = Number.isFinite(raw) ? raw : 0}
			<div
				class="col-span-3 grid cursor-pointer grid-cols-subgrid items-center"
				role="presentation"
				onpointerenter={(e) => {
					hoveredRow = e.currentTarget;
					currentItem = { label, value, row };
				}}
				onpointerleave={(e) => {
					if (e.currentTarget === hoveredRow) {
						hoveredRow = undefined;
					}
				}}
			>
				<span
					class="transition-all max-w-32 @xl/horizontalbarlist:max-w-48 truncate text-xs font-light"
					title={label}>{label}</span
				>
				<progress
					class="progress w-full bg-secondary/30 dark:bg-secondary/50"
					style="height: 12px"
					{value}
					max={maxValue}
					aria-label={label}
				></progress>
				<span class="text-xs font-medium">{formatValue(value)}</span>
			</div>
		{/each}
	</div>
</div>

<style lang="postcss">
	progress {
		color: #4575b4;
		border-top-left-radius: 2px;
		border-bottom-left-radius: 2px;
		border-top-right-radius: 2px;
		border-bottom-right-radius: 2px;
	}

	/* WebKit browsers (Chrome, Safari, Edge) - Background Track */
	progress::-webkit-progress-bar {
		border-top-left-radius: 2px;
		border-bottom-left-radius: 2px;
		border-top-right-radius: 2px;
		border-bottom-right-radius: 2px;
	}

	/* WebKit browsers (Chrome, Safari, Edge) - Filled Value */
	progress::-webkit-progress-value {
		border-top-left-radius: 2px;
		border-bottom-left-radius: 2px;
		border-top-right-radius: 2px;
		border-bottom-right-radius: 2px;
	}

	/* Firefox - Filled Value */
	progress::-moz-progress-bar {
		border-top-left-radius: 2px;
		border-bottom-left-radius: 2px;
		border-top-right-radius: 2px;
		border-bottom-right-radius: 2px;
	}
</style>

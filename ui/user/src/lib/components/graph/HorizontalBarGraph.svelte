<script lang="ts" generics="T extends object">
	import { darkMode } from '$lib/stores';
	import { categoricalColor } from './palette';
	import { chartTooltip } from './tooltip';
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

	// Rows stretch to share the available height but never past this, so a handful of
	// items reads as a tidy ranked list instead of a few slabs filling the panel.
	const MIN_ROW_HEIGHT = 26;
	const MAX_ROW_HEIGHT = 40;

	let clientHeight = $state(0);
	let mounted = $state(false);
	let highlightedBarElement = $state<HTMLElement>();
	let currentItem = $state<TooltipItem>();

	const rows = $derived(
		data.map((row) => {
			const raw = Number(row[valueKey]);
			return { row, label: formatLabel(row[labelKey]), value: Number.isFinite(raw) ? raw : 0 };
		})
	);

	const maxValue = $derived(Math.max(0, ...rows.map((r) => r.value)));

	const rowHeight = $derived(
		rows.length === 0
			? MAX_ROW_HEIGHT
			: Math.max(MIN_ROW_HEIGHT, Math.min(MAX_ROW_HEIGHT, Math.floor(clientHeight / rows.length)))
	);

	const barColor = $derived(categoricalColor(0, darkMode.isDark));

	function percentOfMax(value: number) {
		if (maxValue <= 0 || value <= 0) return 0;
		return (value / maxValue) * 100;
	}

	$effect(() => {
		// Defer one frame so bars grow in from the baseline on first render.
		const id = requestAnimationFrame(() => (mounted = true));
		return () => cancelAnimationFrame(id);
	});
</script>

<div bind:clientHeight class={twMerge('relative h-full w-full min-h-0 min-w-0', klass)}>
	{#if highlightedBarElement && currentItem}
		<div
			class="tooltip bg-base-100 dark:bg-base-300 pointer-events-none fixed top-0 left-0 z-50 flex flex-col shadow-md"
			{@attach (node) => chartTooltip(highlightedBarElement!, node)}
			in:fade={{ duration: 100, delay: 10 }}
			out:fade={{ duration: 100 }}
		>
			{#if tooltipContent}
				{@render tooltipContent(currentItem)}
			{:else}
				<div class="text-muted-content text-xs">{currentItem.label}</div>
				<div class="text-base-content text-lg font-semibold">
					{formatValue(currentItem.value)}
				</div>
			{/if}
		</div>
	{/if}

	<ol
		class="bar-list grid h-full content-start overflow-y-auto"
		style:grid-auto-rows="{rowHeight}px"
	>
		{#each rows as { row, label, value }, i (i)}
			{@const pct = percentOfMax(value)}
			<li
				class="bar-row hover:bg-base-200 dark:hover:bg-base-300 group/row col-span-full grid cursor-default items-center rounded-md px-2 transition-colors duration-150"
				aria-label="{label}: {formatValue(value)}"
				onpointerenter={(e) => {
					const bar = (e.currentTarget as HTMLElement).querySelector<HTMLElement>('[data-bar]');
					highlightedBarElement = bar ?? (e.currentTarget as HTMLElement);
					currentItem = { label, value, row };
				}}
				onpointerleave={() => {
					highlightedBarElement = undefined;
					currentItem = undefined;
				}}
			>
				<span
					class="text-base-content/80 group-hover/row:text-base-content truncate pr-3 text-xs"
					title={label}
				>
					{label}
				</span>
				<span class="bg-base-content/5 relative h-2 min-w-6 rounded-full">
					<span
						data-bar
						class="absolute inset-y-0 left-0 rounded-[2px_4px_4px_2px] transition-[width] duration-700 ease-out"
						style:width={mounted ? (pct > 0 ? `max(${pct}%, 3px)` : '0') : '0'}
						style:background-color={barColor}
						style:transition-delay="{Math.min(i, 12) * 35}ms"
					></span>
				</span>
				<span class="text-base-content pl-3 text-right text-xs font-medium tabular-nums">
					{formatValue(value)}
				</span>
			</li>
		{/each}
	</ol>
</div>

<style>
	.bar-list {
		/* Label column sizes to the longest label, capped so bars always get most of the width. */
		grid-template-columns: fit-content(45%) minmax(0, 1fr) auto;
	}

	.bar-row {
		grid-template-columns: subgrid;
	}
</style>

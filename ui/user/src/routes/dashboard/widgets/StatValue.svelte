<script lang="ts">
	import TweenedMetric from '$lib/components/TweenedMetric.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import { ChevronRight } from '@lucide/svelte';
	import type { Component } from 'svelte';

	type Props = {
		label: string;
		loading: boolean;
		value: number;
		icon: Component<{ class?: string }>;
		format?: (value: number) => string;
	};

	let { label, loading, value, icon: Icon, format }: Props = $props();
</script>

<div class="w-full min-w-0 leading-none">
	<div
		class="text-muted-content mb-1 flex shrink-0 items-center gap-1 text-[11px] tracking-wide @md:text-xs"
	>
		{label}
	</div>

	<div class="flex items-baseline justify-between gap-2">
		{#if loading}
			<Loading class="size-5" />
		{:else}
			<div class="text-lg font-semibold tracking-tight tabular-nums @md:text-xl">
				<TweenedMetric holdAtZero={loading} target={value} {format} />
			</div>
		{/if}
		<div class="relative size-3.5 shrink-0 self-center @md:size-4">
			<Icon
				class="text-primary size-3.5 transition-opacity duration-200 group-hover:opacity-0 @md:size-4"
			/>
			<ChevronRight
				class="text-muted-content pointer-events-none absolute inset-0 size-3.5 opacity-0 transition-opacity duration-200 group-hover:opacity-100 @md:size-4"
			/>
		</div>
	</div>
</div>

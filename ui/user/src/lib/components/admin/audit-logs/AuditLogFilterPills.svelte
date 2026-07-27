<script lang="ts" generics="T">
	import { page } from '$app/state';
	import { goto } from '$lib/url';
	import { X } from '@lucide/svelte';
	import { flip } from 'svelte/animate';
	import { slide } from 'svelte/transition';

	interface Props<T> {
		pillsSearchParamFilters: Record<keyof T, string | undefined | null>;
		getFilterDisplayLabel: (filterKey: keyof T) => string;
		getFilterValue: (filterKey: keyof T, value: string | number) => string;
		isFilterClearable?: (filterKey: keyof T) => boolean;
	}

	const {
		pillsSearchParamFilters,
		getFilterDisplayLabel,
		getFilterValue,
		isFilterClearable
	}: Props<T> = $props();
	const entries = $derived(
		Object.entries(pillsSearchParamFilters) as [keyof T, string | undefined | null][]
	);
</script>

{#if entries.length > 0}
	<div
		class="flex flex-wrap items-center gap-2"
		in:slide={{ duration: 100 }}
		out:slide={{ duration: 50 }}
	>
		{#each entries as [filterKey, filterValues] (filterKey)}
			{@const displayLabel = getFilterDisplayLabel(filterKey)}
			{@const values = filterValues?.toString().split(',').filter(Boolean) ?? []}
			{@const isClearable = isFilterClearable?.(filterKey) ?? true}

			<div class="filter-primary" animate:flip={{ duration: 100 }}>
				<div class="text-xs font-semibold">
					<span>{displayLabel}</span>
					<span>:</span>
					{#each values as value (value)}
						{@const isMultiple = values.length > 1}

						{#if isMultiple}
							<span class="font-light">
								<span>{getFilterValue(filterKey, value)}</span>
							</span>

							<span class="mx-1 font-bold last:hidden">OR</span>
						{:else}
							<span class="font-light">{getFilterValue(filterKey, value)}</span>
						{/if}
					{/each}
				</div>

				{#if isClearable}
					<button
						onclick={() => {
							const url = page.url;
							url.searchParams.set(filterKey.toString(), '');

							goto(url, { noScroll: true });
						}}
					>
						<X class="size-3" />
					</button>
				{/if}
			</div>
		{/each}
	</div>
{/if}

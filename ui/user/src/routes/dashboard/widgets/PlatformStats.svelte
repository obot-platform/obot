<script lang="ts">
	import GraphTitle from '$lib/components/graph/GraphTitle.svelte';
	import { m } from '$lib/i18n';
	import type { PlatformStatId } from '$lib/services/dashboard/layout';
	import { resolve } from '$lib/url';
	import StatValue from './StatValue.svelte';
	import type { Component } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	type Tile = {
		id: PlatformStatId;
		label: string;
		loading: boolean;
		value: number;
		icon: Component<{ class?: string }>;
		seeMore?: string;
		format?: (value: number) => string;
	};

	type Props = {
		tiles: Tile[];
		isBootStrapUser: boolean;
	};

	let { tiles, isBootStrapUser }: Props = $props();

	const fullSet = $derived(tiles.length === 4);
	const linkClasses =
		'col-span-4 flex w-full items-center justify-between gap-2 truncate p-2 group w-full rounded-md transition-colors duration-200 hover:bg-base-300/50';
	const plainClasses = 'col-span-4 flex w-full items-center justify-between gap-2 p-2';

	function cellClass(index: number) {
		if (fullSet) {
			return 'col-span-2 my-2 flex min-w-0 px-2 @sm:col-span-1 @sm:border-r @sm:not-odd:border-r-0 @md:col-span-4 @md:not-last:border-r border-base-300 @xl:col-span-3';
		}
		const span =
			tiles.length <= 1 ? 'col-span-12' : tiles.length === 2 ? 'col-span-6' : 'col-span-4';
		const border = index < tiles.length - 1 ? 'border-r border-base-300' : '';
		return twMerge(span, 'my-2 flex min-w-0 px-2', border);
	}
</script>

<div class="paper flex h-full min-w-0 flex-col gap-0 p-0">
	<GraphTitle title={m.dashboard_on_platform()} element="h4" />
	<div class="@container w-full min-w-0 max-w-full">
		<div
			class={fullSet
				? 'grid w-full grid-cols-2 gap-0 @md:grid-cols-12'
				: 'grid w-full grid-cols-12 gap-0'}
		>
			{#each tiles as tile, index (tile.id)}
				<div class={cellClass(index)}>
					{#if tile.seeMore && !isBootStrapUser}
						<a class={linkClasses} href={resolve(tile.seeMore as `/${string}`)}>
							<StatValue {...tile} />
						</a>
					{:else}
						<div class={plainClasses}>
							<StatValue {...tile} />
						</div>
					{/if}
				</div>
			{/each}
		</div>
	</div>
</div>

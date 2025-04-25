<script lang="ts">
	import { responsive } from '$lib/stores';
	import { onMount } from 'svelte';
	import Search from './Search.svelte';

	interface BaseSearchItem {
		id: string;
		name: string;
		iconURL?: string;
		description?: string;
	}

	interface Props<T extends BaseSearchItem> {
		onSearch: (val: T) => void;
		placeholder?: string;
		items: T[];
		selected?: string[];
		compact?: boolean;
	}

	let {
		onSearch,
		placeholder = 'Search...',
		items,
		selected,
		compact
	}: Props<BaseSearchItem> = $props();

	let searchContainer = $state<HTMLDivElement | null>(null);
	let search = $state('');
	let searchPopover = $state<HTMLDialogElement | null>(null);
	let filteredItems = $derived(
		items.filter(
			(item) =>
				!selected?.includes(item.id) && item.name?.toLowerCase().includes(search.toLowerCase())
		)
	);

	onMount(() => {
		document.addEventListener('click', handleSearchClickOutside);
		return () => {
			document.removeEventListener('click', handleSearchClickOutside);
		};
	});

	function handleSearchClickOutside(event: MouseEvent) {
		if (responsive.isMobile) return;
		if (searchContainer && !searchContainer.contains(event.target as Node) && searchPopover?.open) {
			searchPopover.close();
		}
	}
</script>

<div class="flex grow" bind:this={searchContainer}>
	<div class="relative w-full">
		<Search
			class="dark:border-surface3 border border-transparent bg-white shadow-sm dark:bg-black"
			onChange={(val) => {
				search = val;
			}}
			onMouseDown={() => {
				if (!responsive.isMobile) {
					searchPopover?.show();
				}
			}}
			{placeholder}
			{compact}
		/>

		{@render searchDialog()}
	</div>
</div>

{#snippet searchDialog()}
	<dialog
		bind:this={searchPopover}
		class="default-scrollbar-thin absolute top-12 left-0 z-10 w-full rounded-sm md:max-h-[50vh] md:overflow-y-auto"
		class:hidden={!responsive.isMobile && !search}
	>
		<div class="flex h-full flex-col">
			<div class="default-scrollbar-thin flex min-h-0 grow flex-col overflow-y-auto">
				{#each filteredItems as result}
					{@render searchResult(result)}
				{/each}
				{#if filteredItems.length === 0 && search}
					<p class="px-4 py-2 text-sm font-light text-gray-500">No results found.</p>
				{/if}
			</div>
		</div>
	</dialog>
{/snippet}

{#snippet searchResult(item: BaseSearchItem)}
	<button
		class="hover:bg-surface2 dark:hover:bg-surface3 flex w-full items-center px-4 py-2"
		onclick={(e) => {
			e.stopPropagation();
			onSearch(item);
			searchPopover?.close();
		}}
	>
		{#if item.iconURL}
			<img
				class="size-8 flex-shrink-0 rounded-full bg-white p-1 dark:bg-gray-600"
				src={item.iconURL}
				alt={item.name ?? 'search result'}
			/>
		{/if}
		<span class="flex grow flex-col px-2 text-left">
			<p>
				{item.name}
			</p>
			{#if item.description}
				<span class="text-gray text-xs font-normal dark:text-gray-300">
					{item.description}
				</span>
			{/if}
		</span>
	</button>
{/snippet}

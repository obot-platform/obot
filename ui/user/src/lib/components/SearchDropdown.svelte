<script lang="ts" generics="T extends { id: string | number }">
	import { responsive } from '$lib/stores';
	import { onMount, type Snippet } from 'svelte';
	import Search from './Search.svelte';
	import { twMerge } from 'tailwind-merge';
	import { clickOutside } from '$lib/actions/clickoutside';

	interface Props<T> {
		onSelect: (val: T) => void;
		placeholder?: string;
		items: T[];
		selected?: (string | number)[];
		compact?: boolean;
		renderItem: Snippet<[T]>;
		properties?: (keyof T)[];
		class?: string;
	}

	let {
		onSelect,
		placeholder = 'Search...',
		items,
		selected,
		compact,
		renderItem,
		properties,
		class: klass
	}: Props<T> = $props();

	let searchContainer = $state<HTMLDivElement | null>(null);
	let search = $state('');
	let focused = $state(false);
	let searchPopover = $state<HTMLDialogElement | null>(null);

	let filteredItems = $derived.by(() => {
		if (search.length > 0) {
			return items.filter(
				(item) =>
					!selected?.includes(item.id) &&
					properties?.some(
						(property) =>
							item[property] &&
							item[property]?.toString().toLowerCase().includes(search.toLowerCase())
					)
			);
		}
		return items;
	});

	onMount(() => {
		document.addEventListener('click', handleSearchClickOutside);
		return () => {
			document.removeEventListener('click', handleSearchClickOutside);
		};
	});

	function handleSearchClickOutside(event: MouseEvent) {
		if (responsive.isMobile) return;
		if (searchContainer && !searchContainer.contains(event.target as Node) && searchPopover?.open) {
			searchPopover?.close();
		}
	}
</script>

<div class="flex grow" bind:this={searchContainer}>
	<div class="relative w-full">
		<Search
			class={twMerge(
				'dark:border-surface3 bg-surface1 border border-transparent shadow-inner dark:bg-black',
				klass
			)}
			onChange={(val) => {
				search = val;
			}}
			onMouseDown={() => {
				focused = true;
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
		class:hidden={!responsive.isMobile && !focused}
		use:clickOutside={() => {
			focused = false;
		}}
	>
		<div class="flex h-full flex-col">
			<div class="default-scrollbar-thin flex min-h-0 grow flex-col overflow-y-auto">
				{#each filteredItems as result (result.id)}
					{@render searchResult(result)}
				{/each}
				{#if filteredItems.length === 0 && search}
					<p class="px-4 py-2 text-sm font-light text-gray-500">No results found.</p>
				{/if}
			</div>
		</div>
	</dialog>
{/snippet}

{#snippet searchResult(item: T)}
	<button
		class="hover:bg-surface2 dark:hover:bg-surface3 flex w-full items-center px-4 py-2"
		onclick={(e) => {
			e.stopPropagation();
			onSelect(item);
			searchPopover?.close();
		}}
	>
		{@render renderItem(item)}
	</button>
{/snippet}

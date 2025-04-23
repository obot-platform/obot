<script lang="ts">
	import { clickOutside } from '$lib/actions/clickoutside';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { darkMode, responsive } from '$lib/stores';
	import { ChevronLeft, ChevronRight, ListFilter, Plus, X } from 'lucide-svelte';
	import Search from '../Search.svelte';
	import { type MCP } from '$lib/services';
	import { fade } from 'svelte/transition';
	import McpCard from './McpCard.svelte';

	interface Props {
		inline?: boolean;
		mcps: MCP[];
		onSubmitMcp?: (mcp: MCP) => void;
		submitText?: string;
		selectedMcpIds?: Set<string>;
		hideLogo?: boolean;
	}

	let { inline = false, mcps, onSubmitMcp, submitText, selectedMcpIds, hideLogo }: Props = $props();
	let dialog: HTMLDialogElement | undefined = $state();

	const ITEMS_PER_PAGE = 36;
	let currentPage = $state(1);
	const totalPages = $derived(Math.ceil(mcps.length / ITEMS_PER_PAGE));
	const paginatedMcps = $derived(
		mcps.slice((currentPage - 1) * ITEMS_PER_PAGE, currentPage * ITEMS_PER_PAGE)
	);

	let search = $state('');
	let selectedCategory = $state('Popular');

	const searchResults = $derived(
		mcps.filter((mcp) => mcp.server.name.toLowerCase().includes(search.toLowerCase()))
	);

	let browseAllTitleElement: HTMLDivElement | undefined = $state<HTMLDivElement>();
	let observer: IntersectionObserver;
	let isBrowseAllVisible = $state(true);

	function nextPage() {
		if (currentPage < totalPages) {
			currentPage++;
		}
	}

	function prevPage() {
		if (currentPage > 1) {
			currentPage--;
		}
	}

	function setupObserver() {
		// Always disconnect existing observer before setting up new one
		observer?.disconnect();

		observer = new IntersectionObserver(
			([entry]) => {
				isBrowseAllVisible = entry.isIntersecting;
			},
			{ threshold: 0 }
		);

		if (browseAllTitleElement) {
			observer.observe(browseAllTitleElement);
		}
	}

	$effect(() => {
		if (browseAllTitleElement && !hideLogo) {
			setupObserver();
		}
	});

	const categories = [
		'Popular',
		'Featured',
		'Cloud Platforms',
		'Security & Compliance',
		'Developer Tools',
		'TypeScript',
		'Python',
		'Go',
		'Art & Culture',
		'Analytics & Data',
		'E-commerce',
		'Marketing & Social Media',
		'Productivity',
		'Education'
	];
</script>

{#if inline}
	{@render body()}
{:else}
	<button class="icon-button" onclick={() => dialog?.showModal()} use:tooltip={'Add MCP Server'}>
		<Plus class="size-5" />
	</button>

	<dialog
		bind:this={dialog}
		use:clickOutside={() => dialog?.close()}
		class="default-dialog h-full w-full bg-white pb-4 dark:bg-black"
		class:mobile-screen-dialog={responsive.isMobile}
	>
		<div class="mt-4 flex w-full flex-col items-center justify-center gap-2 px-4 py-4">
			<h2 class="text-3xl font-semibold md:text-4xl">MCP Servers</h2>
			<p class="mb-8 max-w-full text-center text-base font-light md:max-w-md">
				Browse over evergrowing catalog of MCP servers and find the perfect one to set up your agent
				with.
			</p>
		</div>
		<button
			class="icon-button absolute top-4 right-4"
			onclick={() => dialog?.close()}
			use:tooltip={{ disablePortal: true, text: 'Close MCP Servers Catalog' }}
		>
			<X class="size-7" />
		</button>
		{@render body()}
	</dialog>
{/if}

{#snippet body()}
	<div
		class="sticky top-0 left-0 z-30 h-20 w-full max-w-(--breakpoint-2xl) bg-white py-4 dark:bg-black"
	>
		<div class="flex w-full">
			{#if !hideLogo}
				<div class="hidden w-xs pl-4 md:flex">
					{#if !isBrowseAllVisible && !responsive.isMobile}
						<div transition:fade={{ duration: 200 }} class="w-full">
							{@render logo()}
						</div>
					{/if}
				</div>
			{/if}
			<div class="flex w-full items-center gap-4 px-4 md:px-12">
				<Search
					onChange={(val) => {
						search = val;
					}}
					placeholder="Search MCP Servers..."
				/>
				<button
					class="icon-button flex-shrink-0"
					use:tooltip={{ disablePortal: true, text: 'Filter' }}
				>
					<ListFilter class="size-6" />
				</button>
			</div>
		</div>
	</div>

	<div class="relative flex w-full max-w-(--breakpoint-2xl)">
		{#if !responsive.isMobile}
			<div class="sticky top-20 left-0 w-xs p-4" style="height: calc(100vh - 9rem);">
				<div class="flex flex-col gap-4">
					<h3 class="text-2xl font-semibold">Categories</h3>
					<ul class="flex flex-col">
						{#each categories as category}
							<li>
								<button
									class="text-md border-l-3 border-gray-100 px-4 py-2 text-left font-light transition-colors duration-300 dark:border-gray-900"
									class:!border-blue-500={category === selectedCategory}
									onclick={() => {
										selectedCategory = category;
									}}
								>
									{category}
								</button>
							</li>
						{/each}
					</ul>
				</div>
			</div>
		{/if}
		<div class="flex w-full flex-col">
			<div class="flex items-center gap-4 px-4 pt-4 pb-2 md:px-12">
				<h4 bind:this={browseAllTitleElement} class="text-xl font-semibold">
					{search ? 'Search Results' : 'Browse All'}
				</h4>
			</div>
			<div class="grid grid-cols-1 gap-4 px-4 pt-2 md:grid-cols-2 md:px-12 xl:grid-cols-3">
				{#if search}
					{#each searchResults as mcp (mcp.id)}
						<McpCard
							{mcp}
							onSubmit={() => onSubmitMcp?.(mcp)}
							{submitText}
							selected={selectedMcpIds?.has(mcp.id)}
						/>
					{/each}
				{:else}
					{#each paginatedMcps as mcp (mcp.id)}
						<McpCard
							{mcp}
							onSubmit={() => onSubmitMcp?.(mcp)}
							{submitText}
							selected={selectedMcpIds?.has(mcp.id)}
						/>
					{/each}
				{/if}
			</div>
			{#if !search && totalPages > 1}
				<div class="mt-8 flex grow items-center justify-center gap-2">
					<button
						class="button-text flex items-center gap-1 disabled:opacity-50"
						disabled={currentPage === 1}
						onclick={prevPage}
					>
						<ChevronLeft class="size-4" />
						Previous
					</button>
					<span class="text-sm">
						Page {currentPage} of {totalPages}
					</span>
					<button
						class="button-text flex items-center gap-1 disabled:opacity-50"
						disabled={currentPage === totalPages}
						onclick={nextPage}
					>
						Next
						<ChevronRight class="size-4" />
					</button>
				</div>
			{/if}
		</div>
	</div>
{/snippet}

{#snippet logo()}
	<div class="relative flex items-end">
		{#if darkMode.isDark}
			<img src="/user/images/obot-logo-blue-white-text.svg" class="h-12" alt="Obot logo" />
		{:else}
			<img src="/user/images/obot-logo-blue-black-text.svg" class="h-12" alt="Obot logo" />
		{/if}
		<div class="ml-1.5 -translate-y-1">
			<span
				class="rounded-full border-2 border-blue-400 px-1.5 py-[1px] text-[10px] font-bold text-blue-400 dark:border-blue-400 dark:text-blue-400"
			>
				BETA
			</span>
		</div>
	</div>
{/snippet}

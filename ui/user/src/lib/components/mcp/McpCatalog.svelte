<script lang="ts">
	import { clickOutside } from '$lib/actions/clickoutside';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { responsive } from '$lib/stores';
	import { ChevronLeft, ChevronRight, ChevronsRight, X } from 'lucide-svelte';
	import McpCard from '$lib/components/mcp/McpCard.svelte';
	import Search from '$lib/components/Search.svelte';
	import { type MCP } from '$lib/services';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		inline?: boolean;
		mcps: MCP[];
		onSubmitMcp?: (mcpId: string) => void;
		onSubmitMcps?: (mcpIds: string[]) => void;
		selectText?: string;
		submitText?: string;
		cancelText?: string;
		selectedMcpIds?: string[];
		subtitle?: string;
	}

	let {
		inline = false,
		mcps,
		onSubmitMcp,
		onSubmitMcps,
		selectText,
		submitText,
		cancelText,
		selectedMcpIds,
		subtitle
	}: Props = $props();
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

	let selected = $state<string[]>([]);
	const preselected = $derived(new Set(selectedMcpIds ?? []));

	let browseAllTitleElement: HTMLDivElement | undefined = $state<HTMLDivElement>();

	export function open() {
		dialog?.showModal();
	}

	export function getSelectedCount() {
		return selected.length;
	}

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
	<dialog
		bind:this={dialog}
		use:clickOutside={() => dialog?.close()}
		class="default-dialog h-full w-full max-w-(--breakpoint-2xl) bg-white p-0 dark:bg-black"
		class:mobile-screen-dialog={responsive.isMobile}
	>
		<div class="default-scrollbar-thin relative mx-auto h-full min-h-0 w-full overflow-y-auto">
			<button
				class="icon-button sticky top-3 right-2 z-40 float-right self-end"
				onclick={() => dialog?.close()}
				use:tooltip={{ disablePortal: true, text: 'Close MCP Servers Catalog' }}
			>
				<X class="size-7" />
			</button>
			<div class="mt-4 flex w-full flex-col items-center justify-center gap-2 px-4 py-4">
				<h2 class="text-3xl font-semibold md:text-4xl">MCP Servers</h2>
				<p class="mb-8 max-w-full text-center text-base font-light md:max-w-md">
					{subtitle ||
						'Browse over evergrowing catalog of MCP servers and find the perfect one to set up your agent with.'}
				</p>
			</div>
			<div class="pr-12 pb-4">
				{@render body()}
			</div>
			{#if onSubmitMcps}
				<div class="sticky bottom-0 left-0 z-40 w-full bg-white p-4 dark:bg-black">
					<div class="space-between flex items-center justify-end gap-4">
						<span class="text-xs text-gray-300"> Shift+click to quick add</span>
						<button
							class="button-primary flex items-center gap-1"
							onclick={() => {
								onSubmitMcps(selected);
								selected = [];
								dialog?.close();
							}}
							disabled={selected.length === 0}
						>
							{#if selected.length <= 1}
								{submitText || 'Add server'}
							{:else}
								{submitText || `Add ${selected.length} servers`}
							{/if}
							<ChevronsRight class="size-4" />
						</button>
					</div>
				</div>
			{/if}
		</div>
	</dialog>
{/if}

{#snippet body()}
	<div class="relative flex w-full max-w-(--breakpoint-2xl)">
		{#if !responsive.isMobile}
			<div
				class={twMerge(
					'sticky top-0 left-0 h-[calc(100vh-9rem)] w-xs flex-shrink-0 p-4',
					inline && 'h-[50dvh]'
				)}
			>
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
			<div class="sticky top-0 left-0 z-30 w-full">
				<div class="flex grow bg-white p-4 dark:bg-black">
					<Search
						onChange={(val) => {
							search = val;
						}}
						placeholder="Search MCP Servers..."
					/>
				</div>
			</div>
			<div class="flex items-center gap-4 px-4 pt-4 pb-2">
				<h4 bind:this={browseAllTitleElement} class="text-xl font-semibold">
					{search ? 'Search Results' : 'Browse All'}
				</h4>
			</div>
			<div class="grid grid-cols-1 gap-4 px-4 pt-2 md:grid-cols-2 xl:grid-cols-3">
				{#if search}
					{#each searchResults as mcp (mcp.id)}
						{@render mcpCard(mcp)}
					{/each}
				{:else}
					{#each paginatedMcps as mcp (mcp.id)}
						{@render mcpCard(mcp)}
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

{#snippet mcpCard(mcp: MCP)}
	<McpCard
		{mcp}
		onSubmit={() => {
			if (onSubmitMcp) {
				onSubmitMcp(mcp.id);
			} else if (selected.includes(mcp.id)) {
				selected = selected.filter((id) => id !== mcp.id);
			} else {
				selected.push(mcp.id);
			}
		}}
		{selectText}
		{cancelText}
		selected={selected.includes(mcp.id)}
		disabled={preselected.has(mcp.id)}
	/>
{/snippet}

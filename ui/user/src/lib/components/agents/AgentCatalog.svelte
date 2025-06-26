<script lang="ts">
	import { type ProjectTemplate, type MCP, ChatService, type MCPCatalogEntry } from '$lib/services';
	import AgentCard from '$lib/components/agents/AgentCard.svelte';
	import AgentCopy from '$lib/components/agents/AgentCopy.svelte';
	import { sortTemplatesByFeaturedNameOrder } from '$lib/sort';
	import { X, ChevronLeft, ChevronRight, LoaderCircle } from 'lucide-svelte';
	import { clickOutside } from '$lib/actions/clickoutside';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { responsive } from '$lib/stores';
	import Search from '$lib/components/Search.svelte';
	import { onMount } from 'svelte';

	interface Props {
		preselected?: string;
		inline?: boolean;
	}

	let { preselected, inline }: Props = $props();
	let dialog: HTMLDialogElement | undefined = $state();
	let agentCopy: ReturnType<typeof AgentCopy> | undefined = $state();

	let templates = $state<ProjectTemplate[]>([]);
	let loadingTemplates = $state(true);
	let mcps = $state<MCPCatalogEntry[]>([]);
	let loadingMcps = $state(true);

	const ITEMS_PER_PAGE = 36;
	let currentPage = $state(1);

	// Define selectedCategory before using it in filteredTemplates
	let search = $state('');
	let selectedCategory = $state('Featured');

	// Precompute sorted lists for each category
	const categories = ['All', 'Featured', 'Community'];
	const allTemplates = $derived([...templates].sort(sortTemplatesByFeaturedNameOrder));
	const featuredTemplates = $derived(allTemplates.filter((t) => t.featured === true));
	const communityTemplates = $derived(allTemplates.filter((t) => !t.featured));

	// Get the appropriate list based on selected category
	const categoryTemplates = $derived(
		selectedCategory === 'Featured'
			? featuredTemplates
			: selectedCategory === 'Community'
				? communityTemplates
				: allTemplates
	);

	const totalPages = $derived(Math.ceil(categoryTemplates.length / ITEMS_PER_PAGE));
	const paginatedTemplates = $derived(
		categoryTemplates.slice((currentPage - 1) * ITEMS_PER_PAGE, currentPage * ITEMS_PER_PAGE)
	);

	const searchResults = $derived(
		categoryTemplates.filter(
			(t) =>
				t.name?.toLowerCase().includes(search.toLowerCase()) ||
				t.projectSnapshot.name?.toLowerCase().includes(search.toLowerCase()) ||
				t.projectSnapshot.description?.toLowerCase().includes(search.toLowerCase())
		)
	);

	let browseAllTitleElement: HTMLDivElement | undefined = $state<HTMLDivElement>();

	async function loadTemplates() {
		loadingTemplates = true;
		try {
			const result = await ChatService.listTemplates();
			templates = result.items || [];
		} catch (error) {
			console.error('Failed to load templates:', error);
		} finally {
			loadingTemplates = false;
		}
	}

	async function loadMCPs() {
		loadingMcps = true;
		try {
			const results = await ChatService.listMCPs();
			mcps = results;
		} catch (error) {
			console.error('Failed to load MCPs:', error);
		} finally {
			loadingMcps = false;
		}
	}

	export function open() {
		dialog?.showModal();

		// Refresh both templates and MCPs when opening
		loadTemplates();
		loadMCPs();
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

	onMount(() => {
		// Load data when component mounts
		loadTemplates();
		loadMCPs();

		// Handle preselected template
		const preselectedTemplate = preselected && templates.find((t) => t.id === preselected);
		if (preselectedTemplate) {
			const templateMcps =
				(preselectedTemplate?.mcpServers?.map((id) => mcpsMap.get(id)).filter(Boolean) as MCP[]) ||
				[];

			dialog?.showModal();
			agentCopy?.open(preselectedTemplate, templateMcps);
		}
	});

	let mcpsMap = $derived(new Map(mcps.map((m) => [m.id, m])));
</script>

{#if inline}
	{@render content()}
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
				use:tooltip={{ disablePortal: true, text: 'Close Agent Catalog' }}
			>
				<X class="size-7" />
			</button>

			{@render content()}
		</div>
	</dialog>
{/if}

{#snippet content()}
	<div class="mt-4 flex w-full flex-col items-center justify-center gap-2 px-4 py-4">
		<h2 class="text-3xl font-semibold md:text-4xl">Agent Catalog</h2>
		<p class="mb-8 max-w-full text-center text-base font-light md:max-w-md">
			Copy an existing agent to jumpstart your journey
		</p>
	</div>
	<div class="pr-12 pb-4">
		{@render body()}
	</div>
	<AgentCopy bind:this={agentCopy} />
{/snippet}

{#snippet body()}
	<div class="relative flex w-full max-w-(--breakpoint-2xl)">
		{#if !responsive.isMobile}
			<div class={'sticky top-0 left-0 h-[calc(100vh-9rem)] w-xs flex-shrink-0 p-4'}>
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
										currentPage = 1;
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
						placeholder="Search Agents..."
					/>
				</div>
			</div>
			<div class="flex items-center gap-4 px-4 pt-4 pb-2">
				<h4 bind:this={browseAllTitleElement} class="text-xl font-semibold">
					{search ? 'Search Results' : `Browse ${selectedCategory}`}
				</h4>
			</div>
			{#if loadingTemplates || loadingMcps}
				<div class="flex items-center justify-center px-4 pt-2">
					<LoaderCircle class="size-6 animate-spin" />
				</div>
			{:else}
				<div class="grid grid-cols-1 gap-4 px-4 pt-2 md:grid-cols-2 xl:grid-cols-3">
					{#if search}
						{#each searchResults as template (template.id)}
							{@render agentCard(template)}
						{/each}
					{:else}
						{#each paginatedTemplates as template (template.id)}
							{@render agentCard(template)}
						{/each}
					{/if}
				</div>
			{/if}
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

{#snippet agentCard(template: ProjectTemplate)}
	{@const templateMcps =
		(template.mcpServers?.map((id) => mcpsMap.get(id)).filter(Boolean) as MCP[]) || []}

	<AgentCard
		{template}
		mcps={templateMcps}
		onclick={() => {
			agentCopy?.open(template, templateMcps);
		}}
	/>
{/snippet}

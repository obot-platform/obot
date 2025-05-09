<script lang="ts">
	import { clickOutside } from '$lib/actions/clickoutside';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { responsive } from '$lib/stores';
	import { ChevronLeft, ChevronRight, X } from 'lucide-svelte';
	import McpCard from '$lib/components/mcp/McpCard.svelte';
	import Search from '$lib/components/Search.svelte';
	import { type MCP, type MCPManifest, type Project } from '$lib/services';
	import { twMerge } from 'tailwind-merge';
	import McpInfoConfig from '$lib/components/mcp/McpInfoConfig.svelte';
	import type { MCPServerInfo } from '$lib/services/chat/mcp';
	import { getToolBundleMap } from '$lib/context/toolReferences.svelte';

	const BROWSE_ALL_CATEGORY = 'Browse All';

	interface Props {
		inline?: boolean;
		mcps: MCP[];
		onSetupMcp?: (mcpId: string, serverInfo: MCPServerInfo) => void;
		submitText?: string;
		selectedMcpIds?: string[];
		subtitle?: string;
		project?: Project;
	}

	type TransformedMcp = {
		id: string;
		catalogId: string;
		categories: string[];
		manifest: MCPManifest;
		githubStars: number;
		name: string;
	};

	let {
		inline = false,
		mcps: refMcps,
		onSetupMcp,
		submitText,
		selectedMcpIds,
		subtitle,
		project = $bindable()
	}: Props = $props();
	let dialog: HTMLDialogElement | undefined = $state();
	let configDialog = $state<ReturnType<typeof McpInfoConfig>>();
	let selectedMcpManifest = $state<MCPManifest>();

	const toolBundleMap = getToolBundleMap();

	const ITEMS_PER_PAGE = 36;
	let currentPage = $state(1);

	function transformMcp(
		mcp: MCP,
		manifestType: 'command' | 'url',
		manifest: MCPManifest
	): TransformedMcp {
		return {
			id: `${mcp.id}-${manifestType}`,
			catalogId: mcp.id,
			categories: manifest.metadata?.categories?.split(',').map((cat) => cat.trim()) || [],
			manifest,
			githubStars: Number(manifest.githubStars) || 0,
			name: manifest.server?.name ?? ''
		};
	}

	let search = $state('');
	let selectedCategory = $state(BROWSE_ALL_CATEGORY);
	let selectedMcp = $state<TransformedMcp>();
	let legacyBundleId = $derived(
		selectedMcp && toolBundleMap.get(selectedMcp.catalogId) ? selectedMcp.catalogId : undefined
	);

	let transformedMcps: TransformedMcp[] = $derived(
		refMcps
			.flatMap((mcp) => {
				const { commandManifest, urlManifest } = mcp;
				const results: TransformedMcp[] = [];

				if (commandManifest) {
					results.push(transformMcp(mcp, 'command', commandManifest));
				}
				if (urlManifest) {
					results.push(transformMcp(mcp, 'url', urlManifest));
				}
				return results;
			})
			.sort((a, b) => b.githubStars - a.githubStars)
	);

	let filteredMcps: TransformedMcp[] = $derived(
		selectedCategory === BROWSE_ALL_CATEGORY && !search
			? transformedMcps
			: transformedMcps.filter((mcp) => {
					const searchLower = search.toLowerCase();
					const isBrowseAll = selectedCategory === BROWSE_ALL_CATEGORY;

					if (!isBrowseAll && !mcp.categories?.includes(selectedCategory)) {
						return false;
					}
					return !search || mcp.name.toLowerCase().includes(searchLower);
				})
	);

	const totalPages = $derived(Math.ceil(filteredMcps.length / ITEMS_PER_PAGE));

	let paginatedMcps: TransformedMcp[] = $derived(
		filteredMcps.slice((currentPage - 1) * ITEMS_PER_PAGE, currentPage * ITEMS_PER_PAGE)
	);

	let selected = $state<string[]>([]);
	const preselected = $derived(new Set(selectedMcpIds ?? []));

	const categories = $derived(
		Array.from(
			new Set(
				transformedMcps.reduce<string[]>(
					(acc, mcp) => {
						if (mcp.categories?.length) {
							acc.push(...mcp.categories);
						}
						return acc;
					},
					[BROWSE_ALL_CATEGORY]
				)
			)
		)
	);

	export function open() {
		dialog?.showModal();
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
</script>

{#if inline}
	{@render body()}
{:else}
	<dialog
		bind:this={dialog}
		use:clickOutside={() => dialog?.close()}
		class="default-dialog max-w-(calc(100svw - 2em)) h-full w-(--breakpoint-2xl) bg-white p-0 dark:bg-black"
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
		</div>
	</dialog>
{/if}

{#snippet body()}
	<div class="relative flex w-full max-w-(--breakpoint-2xl)">
		{#if !responsive.isMobile}
			<div
				class={twMerge(
					'sticky top-0 left-0 h-[calc(100vh-6rem)] w-xs flex-shrink-0',
					inline && 'h-[50svh]'
				)}
			>
				<div class="flex h-full flex-col gap-4">
					<h3 class="p-4 text-2xl font-semibold">Categories</h3>
					<ul class="default-scrollbar-thin flex min-h-0 grow flex-col overflow-y-auto px-4">
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
							currentPage = 1;
						}}
						placeholder="Search MCP Servers..."
					/>
				</div>
			</div>
			<div class="flex items-center gap-4 px-4 pt-4 pb-2">
				<h4 class="text-xl font-semibold">
					{search ? 'Search Results' : selectedCategory}
				</h4>
			</div>
			<div class="grid grid-cols-1 gap-4 px-4 pt-2 md:grid-cols-2 xl:grid-cols-3">
				{#each paginatedMcps as mcp (mcp.id)}
					{@render mcpCard(mcp)}
				{/each}
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

{#snippet mcpCard(mcp: (typeof transformedMcps)[0])}
	{#if mcp.manifest}
		<McpCard
			tags={mcp.categories}
			manifest={mcp.manifest}
			onSelect={(manifest) => {
				selectedMcp = mcp;
				selectedMcpManifest = manifest;
				configDialog?.open();
			}}
			selected={selected.includes(mcp.id)}
			disabled={preselected.has(mcp.id)}
		/>
	{/if}
{/snippet}

<McpInfoConfig
	bind:this={configDialog}
	bind:project
	manifest={selectedMcpManifest}
	{legacyBundleId}
	onUpdate={(mcpServerInfo) => {
		if (selectedMcp && selectedMcpManifest) {
			onSetupMcp?.(selectedMcp.catalogId, mcpServerInfo);
			dialog?.close();
		}
	}}
	{submitText}
/>

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
	import { onMount } from 'svelte';
	import { dialogAnimation } from '$lib/actions/dialogAnimation';

	const BROWSE_ALL_CATEGORY = 'Browse All';
	const OFFICIAL_CATEGORY = 'Official';
	const VERIFIED_CATEGORY = 'Verified';

	interface Props {
		inline?: boolean;
		mcps: MCP[];
		onSetupMcp?: (mcpId: string, serverInfo: MCPServerInfo) => void;
		submitText?: string;
		selectedMcpIds?: string[];
		subtitle?: string;
		project?: Project;
		preselectedMcp?: string;
	}

	export type TransformedMcp = {
		id: string;
		icon?: string;
		description?: string;
		catalogId: string;
		categories: string[];
		githubStars: number;
		name: string;
		commandManifest?: MCPManifest;
		urlManifest?: MCPManifest;
	};

	let {
		inline = false,
		mcps: refMcps,
		onSetupMcp,
		submitText,
		selectedMcpIds,
		subtitle,
		project = $bindable(),
		preselectedMcp
	}: Props = $props();
	let dialog: HTMLDialogElement | undefined = $state();
	let configDialog = $state<ReturnType<typeof McpInfoConfig>>();
	let selectedMcpManifest = $state<MCPManifest>();
	let searchInput = $state<ReturnType<typeof Search>>();
	let selectManifestDialog = $state<HTMLDialogElement>();

	const toolBundleMap = getToolBundleMap();

	const ITEMS_PER_PAGE = 36;
	let currentPage = $state(1);

	function transformMcp(mcp: MCP): TransformedMcp {
		const { urlManifest, commandManifest } = mcp;
		const githubStars = Math.max(
			Number(commandManifest?.githubStars) || 0,
			Number(urlManifest?.githubStars) || 0
		);
		const categories = Array.from(
			new Set([
				...(commandManifest?.metadata?.categories?.split(',').map((cat) => cat.trim()) || []),
				...(urlManifest?.metadata?.categories?.split(',').map((cat) => cat.trim()) || [])
			])
		);
		const name = commandManifest?.server?.name ?? urlManifest?.server?.name ?? '';
		const icon = commandManifest?.server?.icon ?? urlManifest?.server?.icon ?? '';
		const description =
			commandManifest?.server?.description ?? urlManifest?.server?.description ?? '';

		return {
			id: mcp.id,
			icon,
			description,
			catalogId: mcp.id,
			categories,
			githubStars,
			name,
			commandManifest,
			urlManifest
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
				const results: TransformedMcp[] = [];
				results.push(transformMcp(mcp));
				return results;
			})
			.sort((a, b) => {
				if (selectedCategory === OFFICIAL_CATEGORY) {
					return a.name.localeCompare(b.name);
				}
				return b.githubStars - a.githubStars;
			})
	);

	function getBrowseAllMcps() {
		const { officialMcps, verifiedMcps, rest } = transformedMcps.reduce<{
			officialMcps: TransformedMcp[];
			verifiedMcps: TransformedMcp[];
			rest: TransformedMcp[];
		}>(
			(acc, mcp) => {
				if (mcp.categories?.includes(OFFICIAL_CATEGORY)) {
					acc.officialMcps.push(mcp);
				} else if (mcp.categories?.includes(VERIFIED_CATEGORY)) {
					acc.verifiedMcps.push(mcp);
				} else {
					acc.rest.push(mcp);
				}
				return acc;
			},
			{
				officialMcps: [],
				verifiedMcps: [],
				rest: []
			}
		);
		return [
			...officialMcps.sort((a, b) => a.name.localeCompare(b.name)),
			...verifiedMcps.sort((a, b) => b.githubStars - a.githubStars),
			...rest.sort((a, b) => b.githubStars - a.githubStars)
		];
	}

	let filteredMcps: TransformedMcp[] = $derived(
		selectedCategory === BROWSE_ALL_CATEGORY && !search
			? getBrowseAllMcps()
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
					[BROWSE_ALL_CATEGORY, OFFICIAL_CATEGORY, VERIFIED_CATEGORY]
				)
			)
		)
	);

	onMount(() => {
		const preselectedManifest =
			preselectedMcp && transformedMcps.find((mcp) => mcp.catalogId === preselectedMcp);
		if (preselectedManifest) {
			selectedMcp = preselectedManifest;
			if (preselectedManifest.commandManifest && !preselectedManifest.urlManifest) {
				selectedMcpManifest = preselectedManifest.commandManifest;
			} else if (preselectedManifest.urlManifest && !preselectedManifest.commandManifest) {
				selectedMcpManifest = preselectedManifest.urlManifest;
			}

			if (selectedMcpManifest) {
				configDialog?.open();
			} else {
				selectManifestDialog?.showModal();
			}
		}
	});

	export function open() {
		searchInput?.clear();
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

	function selectManifest(manifestType: 'command' | 'url') {
		if (!selectedMcp) return;
		selectedMcpManifest =
			manifestType === 'command' ? selectedMcp?.commandManifest : selectedMcp?.urlManifest;
		selectManifestDialog?.close();
		configDialog?.open();
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
						bind:this={searchInput}
						onChange={(val) => {
							search = val;
							currentPage = 1;
						}}
						placeholder="Search MCP Servers..."
					/>
				</div>
			</div>

			<div class="flex flex-col gap-1 px-4 pt-4 pb-2">
				<h4 class="text-xl font-semibold">
					{search ? 'Search Results' : selectedCategory}
				</h4>
				<p class="mb-2 text-sm font-light text-gray-500">
					{#if selectedCategory === OFFICIAL_CATEGORY}
						These servers are created and maintained by the Obot team.
					{:else if selectedCategory === VERIFIED_CATEGORY}
						These are open source community servers that have been verified to launch and function
						properly by the Obot team.
					{/if}
				</p>
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

{#snippet mcpCard(mcp: TransformedMcp)}
	<McpCard
		tags={mcp.categories}
		data={mcp}
		onSelect={() => {
			selectedMcp = mcp;
			if (mcp.commandManifest && mcp.urlManifest) {
				selectManifestDialog?.showModal();
			} else {
				selectedMcpManifest = mcp.commandManifest || mcp.urlManifest;
				configDialog?.open();
			}
		}}
		selected={selected.includes(mcp.id)}
		disabled={preselected.has(mcp.id)}
	/>
{/snippet}

<McpInfoConfig
	bind:this={configDialog}
	bind:project
	manifest={selectedMcpManifest}
	manifestType={selectedMcpManifest?.server.command ? 'command' : 'url'}
	{legacyBundleId}
	onUpdate={(mcpServerInfo) => {
		if (selectedMcp && selectedMcpManifest) {
			onSetupMcp?.(selectedMcp.catalogId, mcpServerInfo);
			dialog?.close();
		}
	}}
	{submitText}
/>

<dialog
	class="w-full p-4 pt-0 md:max-w-lg md:p-6 md:pt-4"
	class:mobile-screen-dialog={responsive.isMobile}
	bind:this={selectManifestDialog}
	use:dialogAnimation={{ type: 'fade' }}
	use:clickOutside={() => selectManifestDialog?.close()}
>
	<div class="flex flex-col gap-4">
		<h4
			class="default-dialog-title py-0 text-base"
			class:default-dialog-mobile-title={responsive.isMobile}
		>
			Choose How to Connect
			<button
				class="icon-button md:translate-x-2"
				class:mobile-header-button={responsive.isMobile}
				onclick={() => selectManifestDialog?.close()}
			>
				<X class="size-5" />
			</button>
		</h4>
		<p class="text-sm text-gray-500">
			You can either run this MCP server on Obot or connect to an externally hosted instance.
		</p>
		<div class="flex flex-col items-center justify-center gap-1">
			<button class="button w-full" onclick={() => selectManifest('command')}>Run on Obot</button>
			<span class="text-xs font-light text-gray-500">Let Obot manage the MCP server for you. </span>
		</div>
		{#if selectedMcp?.urlManifest}
			{@const hostname = selectedMcp.urlManifest.server.url?.split('://')[1].split('/')[0]}
			<div class="flex flex-col items-center justify-center gap-1">
				<button class="button w-full" onclick={() => selectManifest('url')}
					>Connect to External Server</button
				>
				<span class="text-xs font-light text-gray-500">
					{#if selectedMcp.urlManifest.server.url}
						Use the preconfigured external server: <b
							class="font-semibold text-black dark:text-white">{hostname}</b
						>
					{:else}
						You'll be asked to specify your MCP URL on the next screen
					{/if}
				</span>
			</div>
		{/if}
	</div>
</dialog>

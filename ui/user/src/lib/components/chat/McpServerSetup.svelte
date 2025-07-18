<script lang="ts">
	import { clickOutside } from '$lib/actions/clickoutside';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { responsive } from '$lib/stores';
	import { ChevronLeft, ChevronRight, LoaderCircle, Server, X } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';
	import Search from '../Search.svelte';
	import {
		ChatService,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type Project,
		type ProjectMCP
	} from '$lib/services';
	import { dialogAnimation } from '$lib/actions/dialogAnimation';
	import { createProjectMcp, parseCategories } from '$lib/services/chat/mcp';
	import PageLoading from '../PageLoading.svelte';
	import McpServerInfo from '../mcp/McpServerInfo.svelte';
	import type { LaunchFormData } from './CatalogConfigureForm.svelte';
	import CatalogConfigureForm from './CatalogConfigureForm.svelte';
	import McpCard from '../mcp/McpCard.svelte';

	interface Props {
		catalogId: string;
		project: Project;
		onSuccess?: (projectMcp: ProjectMCP) => void;
	}

	let { catalogId, project, onSuccess }: Props = $props();

	let servers = $state<MCPCatalogServer[]>([]);
	let entries = $state<MCPCatalogEntry[]>([]);
	let loading = $state(false);
	let configuring = $state(false);

	let catalogDialog = $state<HTMLDialogElement>();
	let infoDialog = $state<HTMLDialogElement>();
	let configDialog = $state<ReturnType<typeof CatalogConfigureForm>>();

	let selectedCategory = $state<string>();
	let searchInput = $state<ReturnType<typeof Search>>();
	let search = $state<string>('');

	let selected = $state<MCPCatalogServer | MCPCatalogEntry>();
	let launchFormData = $state<LaunchFormData>();

	let convertedEntries: (MCPCatalogEntry & { categories: string[] })[] = $derived(
		entries.map((entry) => ({
			...entry,
			categories: parseCategories(entry)
		}))
	);
	let convertedServers: (MCPCatalogServer & { categories: string[] })[] = $derived(
		servers.map((server) => ({
			...server,
			categories: parseCategories(server)
		}))
	);

	let filteredEntriesData = $derived(
		convertedEntries.filter((item) => {
			if (item.deleted) {
				return false;
			}

			if (selectedCategory && !item.categories.includes(selectedCategory)) {
				return false;
			}

			if (search) {
				const nameToUse = item.commandManifest?.name ?? item.urlManifest?.name;
				return nameToUse?.toLowerCase().includes(search.toLowerCase());
			}

			return true;
		})
	);
	let filteredServers = $derived(
		convertedServers.filter((item) => {
			if (item.deleted) {
				return false;
			}

			if (selectedCategory && !item.categories.includes(selectedCategory)) {
				return false;
			}

			if (search) {
				return item.manifest.name?.toLowerCase().includes(search.toLowerCase());
			}

			return true;
		})
	);
	let filteredData = $derived([...filteredServers, ...filteredEntriesData]);

	let page = $state(0);
	let pageSize = $state(30);
	let paginatedData = $derived(filteredData.slice(page * pageSize, (page + 1) * pageSize));

	let categories = $derived([
		...new Set([
			...convertedEntries.flatMap((item) => item.categories),
			...convertedServers.flatMap((item) => item.categories)
		])
	]);

	function closeCatalogDialog() {
		catalogDialog?.close();
		page = 0;
		search = '';
		selectedCategory = undefined;
	}

	function closeInfoDialog() {
		infoDialog?.close();
		selected = undefined;
	}

	export async function open() {
		catalogDialog?.showModal();
		loading = true;
		Promise.all([ChatService.listMCPs(), ChatService.listMCPCatalogServers()])
			.then(([entriesResult, serversResult]) => {
				entries = entriesResult;
				servers = serversResult;
			})
			.finally(() => {
				loading = false;
			});
	}

	function prevPage() {
		page--;
	}

	function nextPage() {
		page++;
	}

	async function handleSetupMcp() {
		if (!selected) return;

		const manifest =
			'urlManifest' in selected || 'commandManifest' in selected
				? (selected.urlManifest ?? selected.commandManifest)
				: (selected as MCPCatalogServer).manifest;
		if (!manifest) return;

		configuring = true;
		const mcpServerInfo = {
			manifest: {
				...manifest,
				url: launchFormData?.url
			},
			env: launchFormData?.envs,
			headers: launchFormData?.headers
		};

		const projectMcp = await createProjectMcp(mcpServerInfo, project);
		onSuccess?.(projectMcp);
		configuring = false;
	}
</script>

<dialog
	bind:this={catalogDialog}
	use:clickOutside={() => closeCatalogDialog()}
	class="default-dialog max-w-(calc(100svw - 2em)) h-full w-(--breakpoint-2xl) bg-gray-50 p-0 dark:bg-black"
	class:mobile-screen-dialog={responsive.isMobile}
>
	<div class="default-scrollbar-thin relative mx-auto h-full min-h-0 w-full overflow-y-auto">
		<button
			class="icon-button sticky top-3 right-2 z-40 float-right self-end"
			onclick={() => closeCatalogDialog()}
			use:tooltip={{ disablePortal: true, text: 'Close' }}
		>
			<X class="size-7" />
		</button>
		<div class="pr-12 pb-4">
			<div class="relative flex w-full max-w-(--breakpoint-2xl)">
				{#if !responsive.isMobile}
					<div class={twMerge('sticky top-0 left-0 h-[calc(100vh-6rem)] w-xs flex-shrink-0')}>
						<div class="flex h-full flex-col gap-4">
							<h3 class="p-4 text-lg font-semibold">MCP Servers</h3>
							<ul class="default-scrollbar-thin flex min-h-0 grow flex-col overflow-y-auto px-4">
								<li>
									<button
										class="text-md border-l-3 border-gray-100 px-4 py-2 text-left font-light transition-colors duration-300 dark:border-gray-900"
										class:!border-blue-500={!selectedCategory}
										onclick={() => {
											selectedCategory = undefined;
											page = 0;
										}}
									>
										Browse All
									</button>
								</li>
								{#each categories as category (category)}
									<li>
										<button
											class="text-md border-l-3 border-gray-100 px-4 py-2 text-left font-light transition-colors duration-300 dark:border-gray-900"
											class:!border-blue-500={category === selectedCategory}
											onclick={() => {
												selectedCategory = category;
												page = 0;
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
						<div class="flex grow bg-gray-50 p-4 dark:bg-black">
							<Search
								class="dark:bg-surface1 dark:border-surface3 bg-white shadow-sm dark:border"
								bind:this={searchInput}
								onChange={(val) => {
									search = val;
									page = 0;
								}}
								placeholder="Search MCP Servers..."
							/>
						</div>
					</div>

					{#if search || selectedCategory}
						<div class="flex flex-col gap-1 px-4 pt-4 pb-2">
							<h4 class="text-xl font-semibold">
								{search ? 'Search Results' : selectedCategory}
							</h4>
						</div>
					{/if}
					{#if loading}
						<div class="flex grow items-center justify-center">
							<LoaderCircle class="size-6 animate-spin" />
						</div>
					{:else}
						<div class="grid grid-cols-1 gap-4 px-4 pt-2 md:grid-cols-2 xl:grid-cols-3">
							{#each paginatedData as mcp (mcp.id)}
								<McpCard
									data={mcp}
									onClick={() => {
										selected = mcp;
										catalogDialog?.close();
										infoDialog?.showModal();
									}}
								/>
							{/each}
						</div>
					{/if}
					{#if !search && filteredData.length > pageSize}
						<div class="mt-8 flex grow items-center justify-center gap-2">
							<button
								class="button-text flex items-center gap-1 disabled:opacity-50"
								disabled={page === 0}
								onclick={prevPage}
							>
								<ChevronLeft class="size-4" />
								Previous
							</button>
							<span class="text-sm">
								Page {page + 1} of {Math.ceil(filteredData.length / pageSize)}
							</span>
							<button
								class="button-text flex items-center gap-1 disabled:opacity-50"
								disabled={page === Math.ceil(filteredData.length / pageSize)}
								onclick={nextPage}
							>
								Next
								<ChevronRight class="size-4" />
							</button>
						</div>
					{/if}
				</div>
			</div>
		</div>
	</div>
</dialog>

<dialog
	bind:this={infoDialog}
	use:clickOutside={() => closeInfoDialog()}
	class="default-dialog max-w-(calc(100svw - 2em)) h-full w-(--breakpoint-2xl) bg-gray-50 p-0 dark:bg-black"
	class:mobile-screen-dialog={responsive.isMobile}
	use:dialogAnimation={{ type: 'slide' }}
>
	<div class="default-scrollbar-thin relative mx-auto h-full min-h-0 w-full overflow-y-auto">
		{#if selected}
			{@const icon =
				'manifest' in selected
					? selected.manifest.icon
					: (selected.commandManifest?.icon ?? selected.urlManifest?.icon)}
			{@const name =
				'manifest' in selected
					? selected.manifest.name
					: (selected.commandManifest?.name ?? selected.urlManifest?.name)}
			<div
				class="dark:bg-surface1 dark:border-surface3 sticky top-0 right-2 left-0 z-40 border-b border-transparent bg-white px-4 py-2 shadow-sm"
			>
				<div class="flex items-center justify-between">
					<div class="flex flex-wrap items-center capitalize">
						<ChevronLeft class="mr-2 size-4" />

						<button
							onclick={() => {
								infoDialog?.close();
								selected = undefined;
								catalogDialog?.showModal();
							}}
							class="button-text flex items-center gap-2 p-0 text-lg font-light"
						>
							MCP Servers
						</button>
						<ChevronLeft class="mx-2 size-4" />
						<span class="text-lg font-light">{name}</span>
					</div>

					<button
						class="icon-button"
						onclick={() => closeInfoDialog()}
						use:tooltip={{ disablePortal: true, text: 'Close' }}
					>
						<X class="size-7" />
					</button>
				</div>
			</div>
			<div class="p-4">
				<div class="mb-4 flex items-center gap-2">
					{#if icon}
						<img
							src={icon}
							alt={name}
							class="bg-surface1 size-10 rounded-md p-1 dark:bg-gray-600"
						/>
					{:else}
						<Server class="bg-surface1 size-10 rounded-md p-1 dark:bg-gray-600" />
					{/if}
					<h1 class="text-2xl font-semibold capitalize">
						{name}
					</h1>
					<div class="flex grow items-center justify-end gap-4">
						<button
							class="button-primary"
							onclick={async () => {
								if (!selected) return;

								const isCatalogEntry = 'urlManifest' in selected || 'commandManifest' in selected;
								if (isCatalogEntry) {
									const manifest =
										(selected as MCPCatalogEntry).commandManifest ??
										(selected as MCPCatalogEntry).urlManifest;
									const needsConfiguration =
										manifest?.hostname ||
										(manifest?.env ?? []).length > 0 ||
										(manifest?.headers ?? []).length > 0;
									if (needsConfiguration) {
										infoDialog?.close();
										configDialog?.open();
										launchFormData = {
											envs:
												manifest?.env?.map((env) => ({
													...env,
													value: ''
												})) ?? [],
											headers:
												manifest?.headers?.map((header) => ({
													...header,
													value: ''
												})) ?? [],
											url: manifest?.hostname ? '' : undefined,
											hostname: manifest?.hostname
										};
										return;
									}
								}

								await handleSetupMcp();
								closeInfoDialog();
							}}
						>
							Connect To Server
						</button>
					</div>
				</div>
				<McpServerInfo {catalogId} entry={selected} />
			</div>
		{/if}
	</div>
</dialog>

<CatalogConfigureForm
	name={selected
		? 'manifest' in selected
			? selected?.manifest?.name
			: (selected?.commandManifest?.name ?? selected?.urlManifest?.name)
		: ''}
	icon={selected
		? 'manifest' in selected
			? selected?.manifest?.icon
			: (selected?.commandManifest?.icon ?? selected?.urlManifest?.icon)
		: ''}
	form={launchFormData}
	onClose={() => {
		launchFormData = undefined;
	}}
	bind:this={configDialog}
>
	{#snippet actions()}
		<div class="flex justify-end gap-2">
			<button
				class="button"
				onclick={() => {
					configDialog?.close();
					infoDialog?.showModal();
				}}
			>
				Go Back
			</button>
			<button
				class="button-primary"
				onclick={async () => {
					await handleSetupMcp();
					launchFormData = undefined;
					configDialog?.close();
				}}
			>
				Launch
			</button>
		</div>
	{/snippet}
</CatalogConfigureForm>

<PageLoading show={configuring} text="Connecting MCP server..." />

<script lang="ts">
	import { clickOutside } from '$lib/actions/clickoutside';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { responsive } from '$lib/stores';
	import { Import, X } from 'lucide-svelte';
	import {
		ChatService,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type MCPServerInstance,
		type Project,
		type ProjectMCP
	} from '$lib/services';
	import { createProjectMcp, parseCategories } from '$lib/services/chat/mcp';
	import MyMcpServers, { type ConnectedServer } from '../mcp/MyMcpServers.svelte';

	interface Props {
		project: Project;
		onSuccess?: (projectMcp: ProjectMCP) => void;
	}

	let { project, onSuccess }: Props = $props();

	let userServerInstances = $state<MCPServerInstance[]>([]);
	let userConfiguredServers = $state<MCPCatalogServer[]>([]);
	let servers = $state<MCPCatalogServer[]>([]);
	let entries = $state<MCPCatalogEntry[]>([]);
	let loading = $state(false);

	let myMcpServers = $state<ReturnType<typeof MyMcpServers>>();
	let catalogDialog = $state<HTMLDialogElement>();
	let selectedCategory = $state<string>();

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
	let convertedUserConfiguredServers: (MCPCatalogServer & { categories: string[] })[] = $derived(
		userConfiguredServers.map((server) => ({
			...server,
			categories: parseCategories(server)
		}))
	);

	let categories = $derived([
		...new Set([
			...convertedEntries.flatMap((item) => item.categories),
			...convertedServers.flatMap((item) => item.categories)
		])
	]);

	function closeCatalogDialog() {
		catalogDialog?.close();
		myMcpServers?.reset();
		selectedCategory = undefined;
	}

	async function setupProjectMcp(connectedServer: ConnectedServer) {
		if (!connectedServer || !connectedServer.server) return;
		const mcpServerInfo = {
			manifest: {
				name: connectedServer.server.manifest.name,
				icon: connectedServer.server.manifest.icon,
				description: connectedServer.server.manifest.description,
				metadata: connectedServer.server.manifest.metadata,
				url: connectedServer.connectURL
			}
		};

		const response = await createProjectMcp(mcpServerInfo, project);
		closeCatalogDialog();
		onSuccess?.(response);
	}

	async function loadData(partialRefresh?: boolean) {
		loading = true;
		try {
			if (partialRefresh) {
				const [singleOrRemoteUserServers, serverInstances] = await Promise.all([
					ChatService.listSingleOrRemoteMcpServers(),
					ChatService.listMcpServerInstances()
				]);

				userConfiguredServers = singleOrRemoteUserServers;
				userServerInstances = serverInstances;
			} else {
				const [singleOrRemoteUserServers, entriesResult, serversResult, serverInstances] =
					await Promise.all([
						ChatService.listSingleOrRemoteMcpServers(),
						ChatService.listMCPs(),
						ChatService.listMCPCatalogServers(),
						ChatService.listMcpServerInstances()
					]);

				userConfiguredServers = singleOrRemoteUserServers;
				entries = entriesResult;
				servers = serversResult;
				userServerInstances = serverInstances;
			}
		} catch (error) {
			console.error('Failed to load data:', error);
		} finally {
			loading = false;
		}
	}

	export async function open() {
		catalogDialog?.showModal();
		loadData();
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
		<div class="pr-18">
			<div class="relative flex w-full max-w-(--breakpoint-2xl)">
				{#if !responsive.isMobile}
					<div class="sticky top-0 left-0 h-[calc(100vh-28px)] w-xs flex-shrink-0">
						<div class="flex h-full flex-col gap-4">
							<ul
								class="default-scrollbar-thin flex min-h-0 grow flex-col overflow-y-auto px-4 py-8"
							>
								<li>
									<button
										class="text-md border-l-3 border-gray-100 px-4 py-2 text-left font-light transition-colors duration-300 dark:border-gray-900"
										class:!border-blue-500={!selectedCategory}
										onclick={() => {
											selectedCategory = undefined;
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
				<div class="relative w-full overflow-hidden pt-8 pb-0 pl-8">
					<MyMcpServers
						bind:this={myMcpServers}
						{userServerInstances}
						userConfiguredServers={convertedUserConfiguredServers}
						servers={convertedServers}
						entries={convertedEntries}
						connectSelectText="Add To Chat"
						{loading}
						{selectedCategory}
						disablePortal
						onConnectServer={(connectedServer) => {
							setupProjectMcp(connectedServer);
						}}
						onConnectedServerCardClick={(connectedServer) => {
							setupProjectMcp(connectedServer);
						}}
						onDisconnect={() => {
							loadData(true);
						}}
					>
						{#snippet connectedServerCardAction(connectedServer)}
							<button
								class="icon-button hover:bg-surface1 dark:hover:bg-surface2 size-6 min-h-auto min-w-auto flex-shrink-0 p-1 hover:text-blue-500"
								use:tooltip={{
									text: 'Add To Chat',
									disablePortal: true,
									classes: ['w-28']
								}}
							>
								<Import class="size-4" />
							</button>
						{/snippet}
					</MyMcpServers>
				</div>
			</div>
		</div>
	</div>
</dialog>

{#snippet connectedActions(connectedServer: ConnectedServer)}
	<button class="menu-button" onclick={() => setupProjectMcp(connectedServer)}>
		Add to Chat
	</button>
{/snippet}

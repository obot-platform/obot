<script lang="ts">
	import { clickOutside } from '$lib/actions/clickoutside';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { mcpServersAndEntries, responsive } from '$lib/stores';
	import { ChevronLeft, X } from 'lucide-svelte';
	import {
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type MCPServerInstance,
		type Project,
		type ProjectMCP
	} from '$lib/services';
	import { getProjectMCPs } from '$lib/context/projectMcps.svelte';
	import Search from '../Search.svelte';
	import { createProjectMcp } from '$lib/services/chat/mcp';
	import McpServerActions from '../mcp/McpServerActions.svelte';
	import { fly } from 'svelte/transition';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import McpServerEntryForm from '../admin/McpServerEntryForm.svelte';
	import ConnectToServer from '../mcp/ConnectToServer.svelte';
	import ChatConnectorsView from './ChatConnectorsView.svelte';

	interface Props {
		project: Project;
		onSuccess?: (projectMcp?: ProjectMCP) => void;
	}

	let { project, onSuccess }: Props = $props();

	const projectMCPs = getProjectMCPs();
	let query = $state('');
	let selected = $state<{
		entry?: MCPCatalogEntry;
		server?: MCPCatalogServer;
	}>();

	let catalogDialog = $state<HTMLDialogElement>();
	let connectToServerDialog = $state<ReturnType<typeof ConnectToServer>>();

	let hasExistingConfigured = $derived(
		Boolean(
			selected?.entry &&
				mcpServersAndEntries.current.userConfiguredServers.find(
					(userConfiguredServer) => userConfiguredServer.catalogEntryID === selected?.entry?.id
				)
		)
	);

	const type = $derived(
		selected?.entry?.manifest.runtime === 'composite'
			? 'composite'
			: selected?.entry?.manifest.runtime === 'remote'
				? 'remote'
				: selected?.server?.catalogEntryID
					? 'single'
					: 'multi'
	);

	function closeCatalogDialog() {
		catalogDialog?.close();
		selected = undefined;
		query = '';
	}

	function getUniqueAlias(serverName: string): string | undefined {
		const existingNames = projectMCPs.items
			.filter((mcp) => !mcp.deleted)
			.flatMap((mcp) => [mcp.name || '', mcp.alias || ''])
			.filter(Boolean)
			.map((name) => name.toLowerCase());

		const nameLower = serverName.toLowerCase();

		// Return undefined if no conflict
		if (!existingNames.includes(nameLower)) {
			return undefined;
		}

		// Generate unique alias with counter
		let counter = 1;
		let candidateAlias: string;
		do {
			candidateAlias = `${serverName} ${counter}`;
			counter++;
		} while (existingNames.includes(candidateAlias.toLowerCase()));

		return candidateAlias;
	}

	async function setupProjectMcp({
		server,
		instance
	}: {
		server?: MCPCatalogServer;
		instance?: MCPServerInstance;
	}) {
		if (!server) return;

		const mcpId = instance ? instance.id : server.id;

		// Check if this server is already added to the project
		const existingMcp = projectMCPs.items.find((mcp) => mcp.mcpID === mcpId && !mcp.deleted);
		if (existingMcp) {
			// Server is already added, no-op
			closeCatalogDialog();
			return;
		}

		// Generate unique alias if there's a naming conflict
		const serverName = server.alias || server.manifest?.name || '';
		const aliasToUse = getUniqueAlias(serverName);

		// Create project MCP with optional alias
		const result = await createProjectMcp(project, mcpId, aliasToUse);
		onSuccess?.(result);
		closeCatalogDialog();
	}

	export async function open() {
		catalogDialog?.showModal();
		mcpServersAndEntries.refreshAll();
	}

	const duration = PAGE_TRANSITION_DURATION;
</script>

<dialog
	bind:this={catalogDialog}
	use:clickOutside={() => closeCatalogDialog()}
	class="default-dialog max-w-(calc(100svw - 2em)) h-full w-(--breakpoint-xl) p-0"
	class:mobile-screen-dialog={responsive.isMobile}
>
	<div class="default-scrollbar-thin relative mx-auto h-full min-h-0 w-full overflow-y-auto">
		<div class="relative flex h-full w-full max-w-(--breakpoint-2xl) flex-col">
			{#if selected}
				{@render selectedContent()}
			{:else}
				{@render mainContent()}
			{/if}
		</div>
	</div>
</dialog>

{#snippet selectedContent()}
	{#if selected}
		<div class="flex items-center justify-between gap-4 py-2 pr-4 pl-2">
			<div class="flex items-center gap-2">
				<button class="icon-button" onclick={() => (selected = undefined)}>
					<ChevronLeft class="size-6" />
				</button>
				<h4 class="text-lg font-semibold">
					{selected.server?.alias ||
						selected.server?.manifest.name ||
						selected.entry?.manifest.name}
				</h4>
			</div>
			<div class="flex items-center gap-2">
				{#if selected.entry}
					<McpServerActions
						entry={selected.entry}
						server={selected.server}
						onConnect={setupProjectMcp}
						skipConnectDialog
						connectOnly
					/>
				{:else}
					<McpServerActions
						server={selected.server}
						onConnect={setupProjectMcp}
						skipConnectDialog
						connectOnly
					/>
				{/if}
			</div>
		</div>
		<div
			class="bg-surface1 dark:bg-background flex h-full flex-col gap-6 p-4"
			in:fly={{ x: 100, delay: duration, duration }}
		>
			<McpServerEntryForm
				entry={selected.entry ? selected.entry : selected.server}
				{type}
				readonly
				entity="workspace"
				{hasExistingConfigured}
				isDialogView
				limitViews={['overview', 'tools']}
			/>
		</div>
	{/if}
{/snippet}

{#snippet mainContent()}
	<div class="w-full px-4 py-2">
		<div class="mb-2 flex items-center justify-between gap-4">
			<h4 class="text-lg font-semibold">Add MCP Server</h4>
			<button
				class="icon-button"
				onclick={() => closeCatalogDialog()}
				use:tooltip={{ disablePortal: true, text: 'Close' }}
			>
				<X class="size-6" />
			</button>
		</div>
		<Search
			class="bg-surface1 dark:border-surface3 border border-transparent shadow-inner"
			value={query}
			onChange={(value) => (query = value)}
			placeholder="Search servers..."
		/>
	</div>
	<div class="rounded-t-md shadow-sm">
		<ChatConnectorsView
			{query}
			onSelect={(data) => (selected = data)}
			onConnect={(data) => {
				catalogDialog?.close();
				connectToServerDialog?.open(data);
			}}
		/>
	</div>
{/snippet}

<ConnectToServer
	bind:this={connectToServerDialog}
	skipConnectDialog
	onConnect={setupProjectMcp}
	userConfiguredServers={mcpServersAndEntries.current.userConfiguredServers}
/>

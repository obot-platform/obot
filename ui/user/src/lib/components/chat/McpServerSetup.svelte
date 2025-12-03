<script lang="ts">
	import { clickOutside } from '$lib/actions/clickoutside';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { mcpServersAndEntries, responsive } from '$lib/stores';
	import { X } from 'lucide-svelte';
	import { type MCPCatalogEntry, type Project, type ProjectMCP } from '$lib/services';
	import { getProjectMCPs } from '$lib/context/projectMcps.svelte';
	import Search from '../Search.svelte';
	import { twMerge } from 'tailwind-merge';
	import ChatRegistriesView from './ChatRegistriesView.svelte';
	import ChatDeploymentsView from './ChatDeploymentsView.svelte';

	interface Props {
		project: Project;
		onSuccess?: (projectMcp?: ProjectMCP) => void;
	}

	let { project, onSuccess }: Props = $props();

	const projectMCPs = getProjectMCPs();
	let query = $state('');
	let tabView = $state<'deployments' | 'registry'>('deployments');
	let selectedEntry = $state<MCPCatalogEntry>();

	let catalogDialog = $state<HTMLDialogElement>();
	function closeCatalogDialog() {
		catalogDialog?.close();
	}

	// async function setupProjectMcp(connectedServer: ConnectedServer) {
	// 	if (!connectedServer || !connectedServer.server) return;

	// 	const mcpId = connectedServer.instance
	// 		? connectedServer.instance.id
	// 		: connectedServer.server.id;

	// 	// Check if this server is already added to the project
	// 	const existingMcp = projectMCPs.items.find((mcp) => mcp.mcpID === mcpId && !mcp.deleted);
	// 	if (existingMcp) {
	// 		// Server is already added, no-op
	// 		closeCatalogDialog();
	// 		return;
	// 	}

	// 	// Generate unique alias if there's a naming conflict
	// 	const serverName = connectedServer.server.manifest?.name || '';
	// 	const aliasToUse = getUniqueAlias(serverName);

	// 	// Create project MCP with optional alias
	// 	const result = await createProjectMcp(project, mcpId, aliasToUse);
	// 	onSuccess?.(result);
	// 	closeCatalogDialog();
	// }

	export async function open() {
		catalogDialog?.showModal();
		mcpServersAndEntries.refreshAll();
	}
</script>

<dialog
	bind:this={catalogDialog}
	use:clickOutside={() => closeCatalogDialog()}
	class="default-dialog max-w-(calc(100svw - 2em)) bg-surface1 dark:bg-background h-full w-(--breakpoint-xl) p-0"
	class:mobile-screen-dialog={responsive.isMobile}
>
	<div class="default-scrollbar-thin relative mx-auto h-full min-h-0 w-full overflow-y-auto">
		<div class="relative flex h-full w-full max-w-(--breakpoint-2xl) flex-col">
			{#if selectedEntry}
				{@render selectedEntryContent()}
			{:else}
				{@render mainContent()}
			{/if}
		</div>
	</div>
</dialog>

{#snippet selectedEntryContent()}
	<!-- todo -->
{/snippet}

{#snippet mainContent()}
	<div class="dark:bg-surface2 bg-background w-full px-4 py-2">
		<div class="mb-2 flex items-center justify-between gap-4">
			<h4 class="text-lg font-semibold">Add Connector</h4>
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
	<div class="dark:bg-surface2 bg-background rounded-t-md shadow-sm">
		<div class="flex w-full">
			<button
				class={twMerge('page-tab max-w-full', tabView === 'deployments' && 'page-tab-active')}
				onclick={() => (tabView = 'deployments')}
			>
				My Servers
			</button>
			<button
				class={twMerge('page-tab max-w-full', tabView === 'registry' && 'page-tab-active')}
				onclick={() => (tabView = 'registry')}
			>
				Registry Entries
			</button>
		</div>

		{#if tabView === 'registry'}
			<ChatRegistriesView
				{query}
				onConnect={(data) => {
					// todo:
				}}
			/>
		{:else if tabView === 'deployments'}
			<ChatDeploymentsView
				{query}
				onSelect={(data) => {
					// todo:
				}}
			/>
		{/if}
	</div>
{/snippet}

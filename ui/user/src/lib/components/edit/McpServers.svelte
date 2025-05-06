<script lang="ts">
	import { getProjectMCPs } from '$lib/context/projectMcps.svelte';
	import { ChatService, type MCP, type Project, type ProjectMCP } from '$lib/services';
	import { PencilLine, Plus, Server, Trash2 } from 'lucide-svelte/icons';
	import McpInfoConfig from '$lib/components/mcp/McpInfoConfig.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import McpCatalog from '$lib/components/mcp/McpCatalog.svelte';
	import { onMount } from 'svelte';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { HELPER_TEXTS } from '$lib/context/helperMode.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import { createProjectMcp } from '$lib/services/chat/mcp';
	import { getLayout, openEditProjectMcp } from '$lib/context/layout.svelte';

	interface Props {
		project: Project;
	}

	let { project }: Props = $props();
	let mcpToShow = $state<ProjectMCP>();
	let toDelete = $state<ProjectMCP>();
	let mcpConfigDialog = $state<ReturnType<typeof McpInfoConfig>>();
	let mcpCatalog = $state<ReturnType<typeof McpCatalog>>();
	let mcps = $state<MCP[]>([]);
	const projectMCPs = getProjectMCPs();
	const selectedMcpIds = $derived(
		projectMCPs.items.reduce<string[]>((acc, mcp) => {
			if (mcp.catalogID !== undefined) acc.push(mcp.catalogID);
			return acc;
		}, [])
	);
	const layout = getLayout();
	onMount(() => {
		ChatService.listMCPs().then((response) => {
			mcps = response;
		});
	});

	async function handleRemoveMcp() {
		if (!project?.assistantID || !project.id || !toDelete) return;
		await ChatService.deleteProjectMCP(project.assistantID, project.id, toDelete.id);
		projectMCPs.items = projectMCPs.items.filter((mcp) => mcp.id !== toDelete?.id);
		toDelete = undefined;
	}
</script>

<CollapsePane
	classes={{ header: 'pl-3 py-2', content: 'p-2' }}
	iconSize={5}
	header="MCP Servers"
	helpText={HELPER_TEXTS.mcpServers}
	open={projectMCPs.items.length > 0}
>
	<div class="flex flex-col gap-2">
		{#if projectMCPs.items.length > 0}
			<div class="flex flex-col">
				{#each projectMCPs.items as mcp}
					<div
						class="group hover:bg-surface3 flex w-full items-center rounded-md transition-colors duration-200"
					>
						<button
							class="flex grow items-center gap-1 py-2 pl-1.5"
							onclick={() => {
								mcpToShow = mcp;
								mcpConfigDialog?.open();
							}}
						>
							<div class="rounded-md bg-gray-50 p-1 dark:bg-gray-600">
								{#if mcp.icon}
									<img src={mcp.icon} class="size-4" alt={mcp.name} />
								{:else}
									<Server class="size-4" />
								{/if}
							</div>
							<p class="w-[calc(100%-24px)] truncate text-left text-xs font-light">
								{mcp.name || 'My Custom Server'}
							</p>
						</button>
						<button
							class="py-2 pr-3 transition-opacity duration-200 group-hover:opacity-100 md:opacity-0"
							onclick={() => (toDelete = mcp)}
						>
							<Trash2 class="size-4" />
						</button>
					</div>
				{/each}
			</div>
		{/if}
		<div class="flex justify-end">
			<DotDotDot class="button flex items-center gap-1 text-xs">
				{#snippet icon()}
					<Plus class="size-4" /> Add MCP Server
				{/snippet}
				<div class="default-dialog flex min-w-max flex-col p-2">
					<button class="menu-button" onclick={() => mcpCatalog?.open()}>
						<Server class="size-4" /> Browse Catalog
					</button>
					<button class="menu-button" onclick={() => openEditProjectMcp(layout)}>
						<PencilLine class="size-4" /> Create Config
					</button>
				</div>
			</DotDotDot>
			<McpCatalog
				bind:this={mcpCatalog}
				{mcps}
				subtitle="Extend your agent's capabilities by adding multiple MCP servers from our evergrowing catalog."
				onSetupMcp={async (mcpId, mcpServerInfo) => {
					const newProjectMcp = await createProjectMcp(mcpServerInfo, project, mcpId);
					projectMCPs.items.push(newProjectMcp);
				}}
				{selectedMcpIds}
			/>
		</div>
	</div>
</CollapsePane>

<McpInfoConfig
	bind:this={mcpConfigDialog}
	manifest={mcpToShow}
	onConfigure={() => {
		if (mcpToShow) {
			openEditProjectMcp(layout, mcpToShow);
			mcpConfigDialog?.close();
		}
	}}
	configureText="Modify server"
/>

<Confirm
	msg={`Are you sure you want to delete MCP server: ${toDelete?.name}?`}
	show={!!toDelete}
	onsuccess={handleRemoveMcp}
	oncancel={() => (toDelete = undefined)}
/>

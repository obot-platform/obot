<script lang="ts">
	import { getProjectMCPs } from '$lib/context/projectMcps.svelte';
	import { ChatService, type MCP, type Project, type ProjectMCP } from '$lib/services';
	import { Trash2 } from 'lucide-svelte/icons';
	import McpConfig from '../mcp/McpConfig.svelte';
	import Confirm from '../Confirm.svelte';
	import McpCatalog from '../mcp/McpCatalog.svelte';
	import { onMount } from 'svelte';

	interface Props {
		project: Project;
	}

	let { project }: Props = $props();
	let mcpToShow = $state<ProjectMCP>();
	let toDelete = $state<ProjectMCP>();
	let mcpConfigDialog = $state<ReturnType<typeof McpConfig>>();
	let mcps = $state<MCP[]>([]);
	const projectMCPs = getProjectMCPs();
	const selectedMcpIds = $derived(new Set(projectMCPs.items.map((mcp) => mcp.catalogID)));

	$effect(() => {
		if (mcpToShow) {
			mcpConfigDialog?.open();
		} else {
			mcpConfigDialog?.close();
		}
	});

	onMount(() => {
		ChatService.listMCPs().then((response) => {
			mcps = response;
		});
	});

	async function handleMcpSubmit(mcp: MCP) {
		const projectMcp = await ChatService.configureProjectMCP(
			project.assistantID,
			project.id,
			mcp.id
		);
		projectMCPs.items = [...projectMCPs.items, projectMcp];
	}
</script>

<div class="flex w-full flex-col" id="sidebar-mcp-servers">
	<div class="mb-1 flex items-center gap-1">
		<p class="text-sm font-semibold">MCP Servers</p>
		<div class="grow"></div>
		<McpCatalog
			{mcps}
			submitText="Add Server To Agent"
			onSubmitMcp={handleMcpSubmit}
			{selectedMcpIds}
		/>
	</div>
	<div>
		{#each projectMCPs.items as mcp}
			<div
				class="group hover:bg-surface3 ransition-colors flex min-h-9 w-full items-center rounded-md duration-300"
			>
				<button
					class="flex grow items-center gap-1 py-2 pl-1.5"
					onclick={() => {
						mcpToShow = mcp;
						mcpConfigDialog?.open();
					}}
				>
					<div class="rounded-md bg-gray-50 p-1 dark:bg-gray-600">
						<img src={mcp.icon} class="size-4" alt={mcp.name} />
					</div>
					<p class="w-[calc(100%-24px)] truncate text-left text-xs font-light">{mcp.name}</p>
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
</div>

{#if mcpToShow}
	<McpConfig bind:this={mcpConfigDialog} mcp={mcpToShow} hideSubmitButton />
{/if}

<Confirm
	msg={`Are you sure you want to delete MCP server: ${toDelete?.name}?`}
	show={!!toDelete}
	onsuccess={async () => {
		if (!project?.assistantID || !project.id || !toDelete) return;
		await ChatService.deleteProjectMCP(project.assistantID, project.id, toDelete.id);

		projectMCPs.items = projectMCPs.items.filter((mcp) => mcp.id !== toDelete?.id);
		toDelete = undefined;
	}}
	oncancel={() => (toDelete = undefined)}
/>

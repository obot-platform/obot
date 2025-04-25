<script lang="ts">
	import { getProjectMCPs } from '$lib/context/projectMcps.svelte';
	import { ChatService, type MCP, type Project, type ProjectMCP } from '$lib/services';
	import { Trash2 } from 'lucide-svelte/icons';
	import McpConfig from '$lib/components/mcp/McpConfig.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import McpCatalog from '$lib/components/mcp/McpCatalog.svelte';
	import { onMount } from 'svelte';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';

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

<CollapsePane classes={{ header: 'pl-3 py-2', content: 'p-2' }} iconSize={5}>
	{#snippet header()}
		<span class="flex grow items-center gap-2 text-start text-sm font-extralight">
			MCP Servers
		</span>
	{/snippet}
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
		{/if}
		<div class="flex justify-end">
			<McpCatalog
				{mcps}
				submitText="Add Server To Agent"
				onSubmitMcp={handleMcpSubmit}
				{selectedMcpIds}
			/>
		</div>
	</div>
</CollapsePane>

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

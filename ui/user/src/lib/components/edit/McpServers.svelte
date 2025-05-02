<script lang="ts">
	import { getProjectMCPs } from '$lib/context/projectMcps.svelte';
	import { ChatService, type MCP, type Project, type ProjectMCP } from '$lib/services';
	import { Plus, Trash2 } from 'lucide-svelte/icons';
	import McpConfig from '$lib/components/mcp/McpConfig.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import McpCatalog from '$lib/components/mcp/McpCatalog.svelte';
	import { onMount } from 'svelte';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { getProjectTools } from '$lib/context/projectTools.svelte';
	import { HELPER_TEXTS } from '$lib/context/helperMode.svelte';

	interface Props {
		project: Project;
	}

	let { project }: Props = $props();
	let mcpToShow = $state<ProjectMCP>();
	let toDelete = $state<ProjectMCP>();
	let mcpConfigDialog = $state<ReturnType<typeof McpConfig>>();
	let mcpCatalog = $state<ReturnType<typeof McpCatalog>>();
	let mcps = $state<MCP[]>([]);
	const projectMCPs = getProjectMCPs();
	const projectTools = getProjectTools();
	const selectedMcpIds = $derived(projectMCPs.items.map((mcp) => mcp.catalogID));

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

	async function handleMcpsSubmit(mcpIds: string[]) {
		let newProjectMcps: ProjectMCP[] = [];

		const updatingTools = [...projectTools.tools];
		for (const mcpId of mcpIds) {
			const projectMcp = await ChatService.configureProjectMCP(
				project.assistantID,
				project.id,
				mcpId
			);
			newProjectMcps.push(projectMcp);

			const matchingIndex = updatingTools.findIndex((tool) => tool.id === mcpId);
			if (matchingIndex !== -1) {
				updatingTools[matchingIndex].enabled = true;
			}
		}

		projectTools.tools = updatingTools;
		await ChatService.updateProjectTools(project.assistantID, project.id, {
			items: updatingTools
		});

		projectMCPs.items = [...projectMCPs.items, ...newProjectMcps];
	}

	async function handleRemoveMcp() {
		if (!project?.assistantID || !project.id || !toDelete) return;

		const updatingTools = [...projectTools.tools];
		const matchingIndex = updatingTools.findIndex((tool) => tool.id === toDelete?.catalogID);
		if (matchingIndex !== -1) {
			updatingTools[matchingIndex].enabled = false;

			projectTools.tools = updatingTools;
			await ChatService.updateProjectTools(project.assistantID, project.id, {
				items: updatingTools
			});
		}

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
			<button class="button flex items-center gap-1 text-xs" onclick={() => mcpCatalog?.open()}>
				<Plus class="size-4" /> Add MCP Server
			</button>
			<McpCatalog
				bind:this={mcpCatalog}
				{mcps}
				submitText="Select server"
				cancelText="Deselect server"
				onSubmitMcps={handleMcpsSubmit}
				subtitle="Extend your agent's capabilities by adding multiple MCP servers from our evergrowing catalog."
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
	onsuccess={handleRemoveMcp}
	oncancel={() => (toDelete = undefined)}
/>

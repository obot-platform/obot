<script lang="ts">
	import { closeAll, closeSidebarConfig, getLayout } from '$lib/context/chatLayout.svelte';
	import { ChatService, type Project, type ProjectMCP } from '$lib/services';
	import { Server, Trash2, X } from 'lucide-svelte';
	import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
	import { getProjectMCPs, validateOauthProjectMcps } from '$lib/context/projectMcps.svelte';
	import McpServerInfoAndTools from '../mcp/McpServerInfoAndTools.svelte';
	import Confirm from '../Confirm.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';

	interface Props {
		mcpServer: ProjectMCP;
		project: Project;
		view?: 'overview' | 'tools';
	}

	let { mcpServer, project, view }: Props = $props();
	const layout = getLayout();
	const projectMcps = getProjectMCPs();
	let showDeleteConfirm = $state(false);

	async function handleRemoveMcp() {
		if (!project?.assistantID || !project.id) return;

		await ChatService.deleteProjectMCP(project.assistantID, project.id, mcpServer.id);
		projectMcps.items = projectMcps.items.filter((mcp) => mcp.id !== mcpServer.id);
		showDeleteConfirm = false;
		closeSidebarConfig(layout);
	}
</script>

<div class="flex h-fit w-full justify-center bg-gray-50 dark:bg-black">
	<div class="h-fit w-full px-4 py-4 md:max-w-[1200px] md:px-8">
		<div class="mb-4 flex items-center gap-2">
			{#if mcpServer.icon}
				<img
					src={mcpServer.icon}
					alt={mcpServer.name}
					class="bg-surface1 size-10 rounded-md p-1 dark:bg-gray-600"
				/>
			{:else}
				<Server class="bg-surface1 size-10 rounded-md p-1 dark:bg-gray-600" />
			{/if}
			<h1 class="text-2xl font-semibold capitalize">
				{mcpServer.name}
			</h1>
			<div class="flex grow justify-end gap-2">
				<button
					class="button-destructive"
					use:tooltip={'Delete'}
					onclick={() => (showDeleteConfirm = true)}
				>
					<Trash2 class="size-4" />
				</button>
				<button class="icon-button" onclick={() => closeSidebarConfig(layout)}>
					<X class="size-6" />
				</button>
			</div>
		</div>
		<McpServerInfoAndTools
			{view}
			entry={mcpServer}
			catalogId={DEFAULT_MCP_CATALOG_ID}
			onAuthenticate={async () => {
				const updatedMcps = await validateOauthProjectMcps(
					project.assistantID,
					project.id,
					projectMcps.items
				);
				if (updatedMcps.length > 0) {
					projectMcps.items = updatedMcps;
				}
			}}
			onProjectToolsUpdate={() => {
				closeAll(layout);
			}}
			{project}
		/>
	</div>
</div>

<Confirm
	msg="Are you sure you want to delete this connector from the project?"
	show={showDeleteConfirm}
	onsuccess={handleRemoveMcp}
	oncancel={() => (showDeleteConfirm = false)}
/>

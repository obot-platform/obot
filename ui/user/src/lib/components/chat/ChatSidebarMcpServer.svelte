<script lang="ts">
	import { closeAll, closeSidebarConfig, getLayout } from '$lib/context/chatLayout.svelte';
	import { ChatService, type Project, type ProjectMCP } from '$lib/services';
	import { ChevronLeft, Server, Trash2 } from 'lucide-svelte';
	import { getProjectMCPs, validateOauthProjectMcps } from '$lib/context/projectMcps.svelte';
	import McpServerInfoAndTools from '../mcp/McpServerInfoAndTools.svelte';
	import Confirm from '../Confirm.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { mcpServersAndEntries, responsive } from '$lib/stores';
	import EditExistingDeployment from '../mcp/EditExistingDeployment.svelte';
	import McpServerActions from '../mcp/McpServerActions.svelte';
	import { findServerAndEntryForProjectMcp } from '$lib/services/chat/mcp';

	interface Props {
		mcpServer: ProjectMCP;
		project: Project;
		view?: 'overview' | 'tools';
	}

	let { mcpServer, project, view }: Props = $props();
	const layout = getLayout();
	const projectMcps = getProjectMCPs();
	let showDeleteConfirm = $state(false);
	let editExistingDialog = $state<ReturnType<typeof EditExistingDeployment>>();

	let { server: matchingConfiguredServer, entry: matchingEntry } = $derived(
		findServerAndEntryForProjectMcp(mcpServer)
	);

	async function handleRemoveMcp() {
		if (!project?.assistantID || !project.id) return;

		await ChatService.deleteProjectMCP(project.assistantID, project.id, mcpServer.id);
		projectMcps.items = projectMcps.items.filter((mcp) => mcp.id !== mcpServer.id);
		showDeleteConfirm = false;
		closeSidebarConfig(layout);
	}

	async function refreshProjectMcps() {
		closeAll(layout);
		projectMcps.items = await ChatService.listProjectMCPs(project.assistantID, project.id);
	}
</script>

<div class="bg-surface1 dark:bg-background flex w-full justify-center">
	<div class="w-full md:max-w-[1200px]">
		{#if !layout.sidebarOpen || responsive.isMobile}
			<div class="flex w-full items-center justify-between gap-2 px-4 pt-4">
				<div class="flex flex-shrink-0 items-center gap-2">
					<button class="icon-button" onclick={() => closeSidebarConfig(layout)}>
						<ChevronLeft class="size-6" />
					</button>
					<h1 class="text-xl font-semibold capitalize">{mcpServer.alias || mcpServer.name}</h1>
				</div>
				<div class="flex flex-shrink-0 items-center gap-2">
					<McpServerActions entry={matchingEntry} server={matchingConfiguredServer} isProjectMcp />
				</div>
			</div>
		{/if}

		<div class="mb-4 flex items-center gap-2 px-4 pt-4">
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
				{mcpServer.alias || mcpServer.name}
			</h1>
			<div class="flex grow justify-end gap-2">
				<button
					class="button-destructive"
					use:tooltip={'Delete'}
					onclick={() => (showDeleteConfirm = true)}
				>
					<Trash2 class="size-4" />
				</button>
			</div>
		</div>
		<McpServerInfoAndTools
			{view}
			entry={mcpServer}
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
			onUpdate={refreshProjectMcps}
			{project}
		/>
	</div>
</div>

<Confirm
	msg="Are you sure you want to delete this MCP server from the project?"
	show={showDeleteConfirm}
	onsuccess={handleRemoveMcp}
	oncancel={() => (showDeleteConfirm = false)}
/>

<EditExistingDeployment bind:this={editExistingDialog} onUpdateConfigure={refreshProjectMcps} />

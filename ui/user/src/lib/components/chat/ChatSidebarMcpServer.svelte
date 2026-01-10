<script lang="ts">
	import { closeAll, closeSidebarConfig, getLayout } from '$lib/context/chatLayout.svelte';
	import { ChatService, type Project, type ProjectMCP } from '$lib/services';
	import { Pencil, Server, Trash2, X } from 'lucide-svelte';
	import { getProjectMCPs, validateOauthProjectMcps } from '$lib/context/projectMcps.svelte';
	import McpServerInfoAndTools from '../mcp/McpServerInfoAndTools.svelte';
	import Confirm from '../Confirm.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { mcpServersAndEntries } from '$lib/stores';
	import EditExistingDeployment from '../mcp/EditExistingDeployment.svelte';
	import { hasEditableConfiguration } from '$lib/services/chat/mcp';

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

	let matchingConfiguredServer = $derived(
		mcpServersAndEntries.current.userConfiguredServers.find((s) => s.id === mcpServer.mcpID) ||
			mcpServersAndEntries.current.servers.find((s) => s.id === mcpServer.mcpID)
	);
	let matchingEntry = $derived(
		matchingConfiguredServer?.catalogEntryID
			? mcpServersAndEntries.current.entries.find(
					(e) => e.id === matchingConfiguredServer?.catalogEntryID
				)
			: undefined
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

<div class="bg-surface1 dark:bg-background flex h-fit w-full justify-center">
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
				{mcpServer.alias || mcpServer.name}
			</h1>
			<div class="flex grow justify-end gap-2">
				{#if matchingConfiguredServer && matchingEntry && hasEditableConfiguration(matchingEntry)}
					<button
						class="button-icon size-12"
						use:tooltip={'Edit Configuration'}
						onclick={() => {
							editExistingDialog?.edit({
								entry: matchingEntry,
								server: matchingConfiguredServer
							});
						}}
					>
						<Pencil class="size-4" />
					</button>
				{/if}
				<button
					class="button-destructive"
					use:tooltip={'Delete'}
					onclick={() => (showDeleteConfirm = true)}
				>
					<Trash2 class="size-4" />
				</button>
				<button class="icon-button size-12" onclick={() => closeSidebarConfig(layout)}>
					<X class="size-6" />
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

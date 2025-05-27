<script lang="ts">
	import { getProjectMCPs } from '$lib/context/projectMcps.svelte';
	import { ChatService, type Project, type ProjectMCP } from '$lib/services';
	import { fetchConfigurationStatuses, type MCPServerInfo } from '$lib/services/chat/mcp';
	import { PencilLine, Plus, Server, Trash2, Wrench, TriangleAlert } from 'lucide-svelte/icons';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import McpInfoConfig from '$lib/components/mcp/McpInfoConfig.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { HELPER_TEXTS } from '$lib/context/helperMode.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import { getLayout, openEditProjectMcp, openMCPServerTools } from '$lib/context/layout.svelte';
	import McpSetupWizard from '$lib/components/mcp/McpSetupWizard.svelte';
	import { getToolBundleMap } from '$lib/context/toolReferences.svelte';
	import { DEFAULT_CUSTOM_SERVER_NAME } from '$lib/constants';

	interface Props {
		project: Project;
		chatbot?: boolean;
	}

	let { project, chatbot = false }: Props = $props();
	let mcpToShow = $state<ProjectMCP>();
	let toDelete = $state<ProjectMCP>();

	let mcpConfigDialog = $state<ReturnType<typeof McpInfoConfig>>();
	let mcpSetupWizard = $state<ReturnType<typeof McpSetupWizard>>();

	const projectMCPs = getProjectMCPs();
	const toolBundleMap = getToolBundleMap();
	const layout = getLayout();

	// Refresh MCP list whenever sidebar config changes (and we're not currently editing an MCP)
	$effect(() => {
		const currentConfig = layout.sidebarConfig;

		if (currentConfig !== 'custom-mcp') {
			setTimeout(() => refreshMcpList(), 100);
		}
	});

	export async function refreshMcpList() {
		if (!project?.assistantID || !project.id) return;

		projectMCPs.items = (await ChatService.listProjectMCPs(project.assistantID, project.id)).filter(
			(projectMcp) => !projectMcp.deleted
		);

		const response = await fetchConfigurationStatuses(
			project,
			projectMCPs.items,
			toolBundleMap,
			chatbot
		);
		projectMCPs.configured = response?.configured || {};
		projectMCPs.requiresConfiguration = response?.requiresConfiguration || {};
	}

	let legacyBundleId = $derived(
		mcpToShow?.catalogID && toolBundleMap.get(mcpToShow.catalogID) ? mcpToShow.catalogID : undefined
	);

	const selectedMcpIds = $derived(
		projectMCPs.items.reduce<string[]>((acc, mcp) => {
			if (mcp.catalogID !== undefined) acc.push(mcp.catalogID);
			return acc;
		}, [])
	);

	async function handleRemoveMcp() {
		if (!project?.assistantID || !project.id || !toDelete) return;

		if (chatbot) {
			if (toDelete.catalogID && toolBundleMap.get(toDelete.catalogID)) {
				await ChatService.deleteProjectLocalCredential(
					project.assistantID,
					project.id,
					toDelete.catalogID
				);
			} else if (toDelete.configured) {
				await ChatService.deconfigureProjectMCP(project.assistantID, project.id, toDelete.id);
			}
		} else {
			await ChatService.deleteProjectMCP(project.assistantID, project.id, toDelete.id);
		}

		await refreshMcpList();
		toDelete = undefined;
	}
</script>

<CollapsePane
	classes={{ header: 'pl-3 py-2', content: 'p-2' }}
	iconSize={5}
	header="MCP Servers"
	helpText={HELPER_TEXTS.mcpServers}
	open={Object.keys(projectMCPs.requiresConfiguration).length > 0 ||
		(!chatbot && projectMCPs.items.length > 0)}
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
								const isLegacyBundleServer = mcp.catalogID && toolBundleMap.get(mcp.catalogID);
								if (isLegacyBundleServer) {
									mcpToShow = mcp;
									mcpConfigDialog?.open();
								} else {
									openEditProjectMcp(layout, mcp, chatbot);
								}
							}}
						>
							<div class="rounded-md bg-gray-50 p-1 dark:bg-gray-600">
								{#if mcp.icon}
									<img src={mcp.icon} class="size-4" alt={mcp.name} />
								{:else}
									<Server class="size-4" />
								{/if}
							</div>
							<p
								class="flex w-[calc(100%-24px)] items-center truncate text-left text-xs font-light"
							>
								{mcp.name || DEFAULT_CUSTOM_SERVER_NAME}
								{#if projectMCPs.requiresConfiguration[mcp.id]}
									<span class="ml-1" use:tooltip={'Configuration Required'}>
										<TriangleAlert
											class="size-4"
											stroke="currentColor"
											fill="none"
											color="orange"
										/>
									</span>
								{/if}
							</p>
						</button>
						{#if !chatbot}
							<DotDotDot
								class="p-0 pr-2.5 transition-opacity duration-200 group-hover:opacity-100 md:opacity-0"
							>
								<div class="default-dialog flex min-w-max flex-col p-2">
									<button class="menu-button" onclick={() => openMCPServerTools(layout, mcp)}>
										<Wrench class="size-4" /> Manage Tools
									</button>
									<button class="menu-button" onclick={() => (toDelete = mcp)}>
										<Trash2 class="size-4" /> Delete
									</button>
								</div>
							</DotDotDot>
						{:else if projectMCPs.configured[mcp.id]}
							<DotDotDot
								class="p-0 pr-2.5 transition-opacity duration-200 group-hover:opacity-100 md:opacity-0"
							>
								<div class="default-dialog flex min-w-max flex-col p-2">
									<button class="menu-button" onclick={() => (toDelete = mcp)}>
										<Trash2 class="size-4" /> Delete My Configuration
									</button>
								</div>
							</DotDotDot>
						{/if}
					</div>
				{/each}
			</div>
		{/if}
		{#if !chatbot}
			<div class="flex justify-end">
				<DotDotDot class="button flex items-center gap-1 text-xs">
					{#snippet icon()}
						<Plus class="size-4" /> Add MCP Server
					{/snippet}
					<div class="default-dialog flex min-w-max flex-col p-2">
						<button class="menu-button" onclick={() => mcpSetupWizard?.open()}>
							<Server class="size-4" /> Browse Catalog
						</button>
						<button class="menu-button" onclick={() => openEditProjectMcp(layout)}>
							<PencilLine class="size-4" /> Create Config
						</button>
					</div>
				</DotDotDot>
				<McpSetupWizard
					bind:this={mcpSetupWizard}
					catalogDescription="Extend your agent's capabilities by adding multiple MCP servers from our evergrowing catalog."
					catalogSubmitText="Add to agent"
					{selectedMcpIds}
					{project}
					onFinish={(newProjectMcp) => {
						if (newProjectMcp) {
							projectMCPs.items.push(newProjectMcp);
						}
						mcpSetupWizard?.close();
					}}
				/>
			</div>
		{/if}
	</div>
</CollapsePane>

<McpInfoConfig
	bind:this={mcpConfigDialog}
	manifest={mcpToShow}
	{project}
	{legacyBundleId}
	submitText={legacyBundleId ? 'Reauthenticate' : 'Modify server'}
	legacyAuthText="You will be prompted to login again to reauthenticate."
	onUpdate={async (manifest: MCPServerInfo) => {
		if (!project?.assistantID || !project.id || !mcpToShow) return;

		if (!legacyBundleId) {
			await ChatService.updateProjectMCP(project.assistantID, project.id, mcpToShow.id, manifest);
		}

		await refreshMcpList();
		mcpConfigDialog?.close();
	}}
/>

<Confirm
	msg={chatbot
		? `Are you sure you want to delete your MCP server configuration?`
		: `Are you sure you want to delete MCP server: ${toDelete?.name}?`}
	show={!!toDelete}
	onsuccess={handleRemoveMcp}
	oncancel={() => (toDelete = undefined)}
/>

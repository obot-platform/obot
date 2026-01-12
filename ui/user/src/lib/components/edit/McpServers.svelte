<script lang="ts">
	import { getProjectMCPs, validateOauthProjectMcps } from '$lib/context/projectMcps.svelte';
	import { ChatService, type Project, type ProjectMCP } from '$lib/services';
	import { Server, TriangleAlert, Plus, Pencil, Trash2 } from 'lucide-svelte/icons';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import { closeSidebarConfig, getLayout, openMCPServer } from '$lib/context/chatLayout.svelte';
	import { DEFAULT_CUSTOM_SERVER_NAME } from '$lib/constants';
	import McpServerSetup from '../chat/McpServerSetup.svelte';
	import DotDotDot from '../DotDotDot.svelte';
	import { mcpServersAndEntries } from '$lib/stores';
	import EditExistingDeployment from '../mcp/EditExistingDeployment.svelte';
	import { hasEditableConfiguration } from '$lib/services/chat/mcp';

	interface Props {
		project: Project;
	}

	let { project }: Props = $props();
	let toDelete = $state<ProjectMCP>();
	let loading = $state(false);

	let mcpServerSetup = $state<ReturnType<typeof McpServerSetup>>();
	let editExistingDialog = $state<ReturnType<typeof EditExistingDeployment>>();

	const projectMCPs = getProjectMCPs();
	const layout = getLayout();

	let configuredServersMap = $derived(
		new Map(
			[
				...mcpServersAndEntries.current.userConfiguredServers,
				...mcpServersAndEntries.current.servers
			].map((s) => [s.id, s])
		)
	);

	let entriesMap = $derived(new Map(mcpServersAndEntries.current.entries.map((e) => [e.id, e])));

	// Refresh MCP list whenever sidebar config changes (and we're not currently editing an MCP)
	$effect(() => {
		setTimeout(() => refreshMcpList(), 100);
	});

	export async function refreshMcpList() {
		if (!project?.assistantID || !project.id) return;

		const existingAuthenticatedMap = projectMCPs.items.reduce<Record<string, boolean>>(
			(acc, mcp) => {
				if (mcp.authenticated) {
					acc[mcp.id!] = mcp.authenticated;
				}
				return acc;
			},
			{}
		);

		const data = (await ChatService.listProjectMCPs(project.assistantID, project.id)).filter(
			(projectMcp) => !projectMcp.deleted
		);

		const dataWithExistingAuthenticated = data.map((mcp) => {
			if (existingAuthenticatedMap[mcp.id]) {
				return { ...mcp, authenticated: existingAuthenticatedMap[mcp.id] };
			}
			return mcp;
		});

		const updatedMcps = await validateOauthProjectMcps(
			project.assistantID,
			project.id,
			dataWithExistingAuthenticated
		);
		projectMCPs.items = updatedMcps.length > 0 ? updatedMcps : dataWithExistingAuthenticated;
	}

	function shouldShowWarning(mcp: (typeof projectMCPs.items)[0]) {
		if (mcp.needsURL) {
			return true;
		}

		if (typeof mcp.configured === 'boolean' && mcp.configured === false) {
			return true;
		}

		if (typeof mcp.authenticated === 'boolean') {
			return !mcp.authenticated;
		}

		return !!mcp.oauthURL;
	}

	function warningTooltip(mcp: (typeof projectMCPs.items)[0]) {
		if (mcp.needsURL) return 'Configuration Required';
		if (typeof mcp.configured === 'boolean' && mcp.configured === false)
			return 'Configuration Required';
		if (typeof mcp.authenticated === 'boolean' && mcp.authenticated === false)
			return 'Authentication Required';
		if (mcp.oauthURL) return 'Authentication Required';
		return 'Configuration Required';
	}

	async function handleRemoveMcp() {
		if (!project?.assistantID || !project.id || !toDelete) return;
		loading = true;

		if (layout.mcpServer?.id === toDelete.id) {
			closeSidebarConfig(layout);
		}

		await ChatService.deleteProjectMCP(project.assistantID, project.id, toDelete.id);
		await refreshMcpList();
		toDelete = undefined;
		loading = false;
	}
</script>

<div class="flex flex-col text-xs">
	<div class="flex items-center justify-between">
		<p class="text-md grow font-medium">MCP Servers</p>
		<button
			class="hover:text-on-background text-on-surface1 p-2 transition-colors duration-200"
			onclick={() => mcpServerSetup?.open()}
			use:tooltip={'Add MCP Server'}
		>
			<Plus class="h-5 w-5" />
		</button>
	</div>
	{#if projectMCPs.items.length > 0}
		<div class="flex flex-col">
			{#each projectMCPs.items as mcpServer (mcpServer.id)}
				{@const matchingConfiguredServer = configuredServersMap.get(mcpServer.mcpID)}
				{@const matchingEntry = matchingConfiguredServer?.catalogEntryID
					? entriesMap.get(matchingConfiguredServer?.catalogEntryID)
					: undefined}
				<div
					class="group hover:bg-surface3 flex w-full items-center rounded-md transition-colors duration-200"
				>
					<button
						class="flex grow items-center gap-1 py-2 pl-1.5"
						onclick={() => {
							openMCPServer(layout, mcpServer);
						}}
					>
						<div class="rounded-md bg-gray-50 p-1 dark:bg-gray-600">
							{#if mcpServer.icon}
								<img src={mcpServer.icon} class="size-4" alt={mcpServer.alias || mcpServer.name} />
							{:else}
								<Server class="size-4" />
							{/if}
						</div>
						<p
							class="flex w-[calc(100%-24px)] items-center truncate pl-1.5 text-left text-xs font-light"
						>
							{mcpServer.alias || mcpServer.name || DEFAULT_CUSTOM_SERVER_NAME}
							{#if shouldShowWarning(mcpServer)}
								<span class="ml-1" use:tooltip={warningTooltip(mcpServer)}>
									<TriangleAlert class="size-3" stroke="currentColor" fill="none" color="orange" />
								</span>
							{/if}
						</p>
					</button>

					<DotDotDot
						class="p-0 pr-2.5 transition-opacity duration-200 group-hover:opacity-100 md:opacity-0"
					>
						<div class="default-dialog flex min-w-max flex-col p-2">
							{#if matchingEntry && matchingConfiguredServer && hasEditableConfiguration(matchingEntry)}
								<button
									class="menu-button"
									onclick={() => {
										editExistingDialog?.edit({
											server: matchingConfiguredServer,
											entry: matchingEntry
										});
									}}
								>
									<Pencil class="h-4 w-4" /> Edit Configuration
								</button>
							{/if}
							<button class="menu-button" onclick={() => (toDelete = mcpServer)}>
								<Trash2 class="h-4 w-4" /> Delete
							</button>
						</div>
					</DotDotDot>
				</div>
			{/each}
		</div>
	{/if}

	<McpServerSetup bind:this={mcpServerSetup} {project} onSuccess={() => refreshMcpList()} />
</div>

<Confirm
	msg="Are you sure you want to delete your MCP server configuration?"
	show={!!toDelete}
	onsuccess={handleRemoveMcp}
	oncancel={() => (toDelete = undefined)}
	{loading}
/>

<EditExistingDeployment bind:this={editExistingDialog} onUpdateConfigure={refreshMcpList} />

<script lang="ts">
	import { getProjectMCPs, validateOauthProjectMcps } from '$lib/context/projectMcps.svelte';
	import { ChatService, type Project, type ProjectMCP } from '$lib/services';
	import { Server, TriangleAlert, Plus } from 'lucide-svelte/icons';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import { getLayout, openMCPServer } from '$lib/context/chatLayout.svelte';
	import { DEFAULT_CUSTOM_SERVER_NAME } from '$lib/constants';
	import McpServerSetup from '../chat/McpServerSetup.svelte';
	import McpServerActions from '../chat/McpServerActions.svelte';

	interface Props {
		project: Project;
		chatbot?: boolean;
	}

	let { project, chatbot = false }: Props = $props();
	let toDelete = $state<ProjectMCP>();
	let loading = $state(false);

	let mcpServerSetup = $state<ReturnType<typeof McpServerSetup>>();
	const projectMCPs = getProjectMCPs();
	const layout = getLayout();

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
		if (typeof mcp.authenticated === 'boolean' && !mcp.authenticated) {
			return true;
		}
	}

	async function handleRemoveMcp() {
		if (!project?.assistantID || !project.id || !toDelete) return;
		loading = true;
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
			class="icon-button"
			onclick={() => mcpServerSetup?.open()}
			use:tooltip={'Add MCP Server'}
		>
			<Plus class="h-5 w-5" />
		</button>
	</div>
	{#if projectMCPs.items.length > 0}
		<div class="flex flex-col">
			{#each projectMCPs.items as mcpServer (mcpServer.id)}
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
								<img src={mcpServer.icon} class="size-4" alt={mcpServer.name} />
							{:else}
								<Server class="size-4" />
							{/if}
						</div>
						<p class="flex w-[calc(100%-24px)] items-center truncate text-left text-xs font-light">
							{mcpServer.name || DEFAULT_CUSTOM_SERVER_NAME}
							{#if shouldShowWarning(mcpServer)}
								<span
									class="ml-1"
									use:tooltip={mcpServer.authenticated
										? 'Configuration Required'
										: 'Authentication Required'}
								>
									<TriangleAlert class="size-4" stroke="currentColor" fill="none" color="orange" />
								</span>
							{/if}
						</p>
					</button>
					{#if !chatbot}
						<McpServerActions
							class="p-0 pr-2.5 transition-opacity duration-200 group-hover:opacity-100 md:opacity-0"
							{mcpServer}
							{project}
							onDelete={() => refreshMcpList()}
						/>
					{/if}
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

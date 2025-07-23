<script lang="ts">
	import { getProjectMCPs, validateOauthProjectMcps } from '$lib/context/projectMcps.svelte';
	import {
		ChatService,
		type Project,
		type ProjectMCP,
		type ProjectCredential
	} from '$lib/services';
	import { Server, Trash2, TriangleAlert, Plus } from 'lucide-svelte/icons';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import { onMount } from 'svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import { getLayout, openMCPServer } from '$lib/context/chatLayout.svelte';
	import { getToolBundleMap } from '$lib/context/toolReferences.svelte';
	import { DEFAULT_CUSTOM_SERVER_NAME } from '$lib/constants';
	import { errors } from '$lib/stores';
	import McpServerSetup from '../chat/McpServerSetup.svelte';
	import McpServerActions from '../chat/McpServerActions.svelte';
	import { HELPER_TEXTS } from '$lib/context/helperMode.svelte';
	import CollapsePane from './CollapsePane.svelte';

	interface Props {
		project: Project;
		chatbot?: boolean;
	}

	let { project, chatbot = false }: Props = $props();
	let toDelete = $state<ProjectMCP>();
	let localCredentials = $state<ProjectCredential[]>([]);
	let inheritedCredentials = $state<ProjectCredential[]>([]);
	let localConfigurations = $state<Record<string, boolean>>({});
	let loading = $state(false);

	let mcpServerSetup = $state<ReturnType<typeof McpServerSetup>>();
	const projectMCPs = getProjectMCPs();
	const toolBundleMap = getToolBundleMap();
	const layout = getLayout();

	// Refresh MCP list whenever sidebar config changes (and we're not currently editing an MCP)
	$effect(() => {
		setTimeout(() => refreshMcpList(), 100);
	});

	onMount(() => {
		if (project?.assistantID && project.id && chatbot) {
			fetchCredentials();
		}
	});

	export async function refreshMcpList() {
		if (!project?.assistantID || !project.id) return;

		const existingAuthenticatedMap = projectMCPs.items.reduce<Record<string, boolean>>(
			(acc, mcp) => {
				if (mcp.authenticated) {
					acc[mcp.id] = mcp.authenticated;
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

		const updatedMcps = await validateOauthProjectMcps(dataWithExistingAuthenticated);
		projectMCPs.items = updatedMcps.length > 0 ? updatedMcps : dataWithExistingAuthenticated;

		await fetchCredentials();
	}

	async function fetchCredentials() {
		if (!project?.assistantID || !project.id) return;

		try {
			localCredentials = (
				await ChatService.listProjectLocalCredentials(project.assistantID, project.id)
			).items;

			inheritedCredentials = (
				await ChatService.listProjectCredentials(project.assistantID, project.id)
			).items;

			localConfigurations = {};
			for (const mcp of projectMCPs.items) {
				localConfigurations[mcp.id] = await hasLocalConfig(mcp);
			}
		} catch (error) {
			console.error('Failed to fetch credentials:', error);
		}
	}

	async function hasLocalConfig(mcp: ProjectMCP): Promise<boolean> {
		// Handle legacy tool bundles
		if (mcp.catalogEntryID && toolBundleMap.get(mcp.catalogEntryID)) {
			return localCredentials.some(
				(cred) => cred.toolID === mcp.catalogEntryID && cred.exists === true
			);
		}

		// Real MCP server, reveal any configured env headers
		let envHeaders: Record<string, string> = {};
		try {
			envHeaders = await ChatService.revealProjectMCPEnvHeaders(
				project.assistantID,
				project.id,
				mcp.id
			);
		} catch (err) {
			if (err instanceof Error && err.message.includes('404')) {
				return false;
			}

			errors.append(err);
		}

		return Object.keys(envHeaders).length > 0;
	}

	function shouldShowWarning(mcp: (typeof projectMCPs.items)[0]) {
		if (!mcp.authenticated) {
			return true;
		}

		if (!mcp.catalogEntryID || !toolBundleMap.get(mcp.catalogEntryID)) {
			return mcp.configured !== true;
		}

		const localCredential = localCredentials.find((cred) => cred.toolID === mcp.catalogEntryID);

		if (localCredential === undefined) {
			// When there's no entry in this list, it means the tool does not require credentials.
			return false;
		}

		const hasLocalCredential = localCredential.exists;
		if (chatbot) {
			return !hasLocalCredential;
		}

		const hasInheritedCredential = inheritedCredentials.some(
			(cred) => cred.toolID === mcp.catalogEntryID && cred.exists === true
		);

		return !(hasLocalCredential || hasInheritedCredential);
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

<CollapsePane
	classes={{ header: 'pl-3 py-2', content: 'p-2' }}
	iconSize={5}
	header="MCP Servers"
	helpText={HELPER_TEXTS.mcpServers}
	open={projectMCPs.items.some(shouldShowWarning) || (!chatbot && projectMCPs.items.length > 0)}
>
	<div class="flex flex-col gap-2">
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
								{#if mcpServer.manifest.icon}
									<img src={mcpServer.manifest.icon} class="size-4" alt={mcpServer.manifest.name} />
								{:else}
									<Server class="size-4" />
								{/if}
							</div>
							<p
								class="flex w-[calc(100%-24px)] items-center truncate text-left text-xs font-light"
							>
								{mcpServer.manifest.name || DEFAULT_CUSTOM_SERVER_NAME}
								{#if shouldShowWarning(mcpServer)}
									<span
										class="ml-1"
										use:tooltip={mcpServer.authenticated
											? 'Configuration Required'
											: 'Authentication Required'}
									>
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
							<McpServerActions
								class="p-0 pr-2.5 transition-opacity duration-200 group-hover:opacity-100 md:opacity-0"
								{mcpServer}
								{project}
								onDelete={() => refreshMcpList()}
							/>
						{:else if localConfigurations[mcpServer.id]}
							<DotDotDot
								class="p-0 pr-2.5 transition-opacity duration-200 group-hover:opacity-100 md:opacity-0"
							>
								<div class="default-dialog flex min-w-max flex-col p-2">
									<button class="menu-button" onclick={() => (toDelete = mcpServer)}>
										<Trash2 class="size-4" /> Delete My Configuration
									</button>
								</div>
							</DotDotDot>
						{/if}
					</div>
				{/each}
			</div>
		{/if}

		<div class="flex justify-end">
			<button class="button flex items-center gap-1 text-xs" onclick={() => mcpServerSetup?.open()}>
				<Plus class="size-4" /> Add MCP Server
			</button>
			<McpServerSetup bind:this={mcpServerSetup} {project} onSuccess={() => refreshMcpList()} />
		</div>
	</div>
</CollapsePane>

<Confirm
	msg="Are you sure you want to delete your MCP server configuration?"
	show={!!toDelete}
	onsuccess={handleRemoveMcp}
	oncancel={() => (toDelete = undefined)}
	{loading}
/>

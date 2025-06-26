<script lang="ts">
	import {
		ChatService,
		EditorService,
		type MCPServerTool,
		type Project,
		type ProjectMCP
	} from '$lib/services';
	import { createProjectMcp, type MCPServerInfo, updateProjectMcp } from '$lib/services/chat/mcp';
	import McpCatalog, { type TransformedMcp } from '$lib/components/mcp/McpCatalog.svelte';
	import McpInfoConfig from '$lib/components/mcp/McpInfoConfig.svelte';
	import { goto } from '$app/navigation';
	import ProjectMcpServerTools from './ProjectMcpServerTools.svelte';
	import { responsive } from '$lib/stores';
	import { clickOutside } from '$lib/actions/clickoutside';
	import { getToolBundleMap } from '$lib/context/toolReferences.svelte';
	import PageLoading from '$lib/components/PageLoading.svelte';

	interface Props {
		catalogDescription?: string;
		catalogSubmitText?: string;
		inline?: boolean;
		onFinish?: (projectMcp?: ProjectMCP, project?: Project) => void;
		project?: Project;
		selectedMcpIds?: string[];
		preselected?: string;
	}

	let {
		catalogDescription,
		catalogSubmitText,
		inline,
		onFinish,
		project: refProject,
		selectedMcpIds,
		preselected
	}: Props = $props();
	let project = $state(refProject);
	let selectedMcp = $state<TransformedMcp>();
	let projectMcp = $state<ProjectMCP>();
	let projectMcpServerInfo = $state<MCPServerInfo>();
	let projectMcpServerTools = $state<MCPServerTool[]>([]);

	let mcpCatalog = $state<ReturnType<typeof McpCatalog>>();
	let mcpInfoConfig = $state<ReturnType<typeof McpInfoConfig>>();
	let projectMcpServerToolsDialog = $state<HTMLDialogElement>();

	const toolBundleMap = getToolBundleMap();
	let legacyBundleId = $derived(
		projectMcp?.catalogEntryID && toolBundleMap.get(projectMcp.catalogEntryID)
			? projectMcp.catalogEntryID
			: undefined
	);

	let processing = $state(false);

	export function open() {
		mcpCatalog?.open();

		// reset on open
		projectMcp = undefined;
		projectMcpServerInfo = undefined;
		projectMcpServerTools = [];
	}

	export function close() {
		mcpInfoConfig?.close();
		projectMcpServerToolsDialog?.close();
	}

	async function setup(mcpServerInfo: MCPServerInfo, mcpId?: string) {
		processing = true;
		try {
			if (!project) {
				project = await EditorService.createObot();
			}

			projectMcpServerInfo = mcpServerInfo;
			projectMcp = projectMcp
				? await updateProjectMcp(mcpServerInfo, projectMcp.id, project)
				: await createProjectMcp(mcpServerInfo, project, mcpId);

			if (!projectMcp.configured) {
				processing = false;
				await new Promise((resolve) => setTimeout(resolve, 200));
				mcpInfoConfig?.open();
				return;
			}

			projectMcpServerTools = await ChatService.listProjectMCPServerTools(
				project.assistantID,
				project.id,
				projectMcp.id
			);

			processing = false;
			projectMcpServerToolsDialog?.showModal();
		} catch (error) {
			console.error('error occurred during agent mcp server setup', error);
			processing = false;
			await new Promise((resolve) => setTimeout(resolve, 200));
			mcpInfoConfig?.open();
		}
	}
</script>

<McpCatalog
	bind:this={mcpCatalog}
	bind:project
	{inline}
	subtitle={catalogDescription}
	onSetupMcp={(mcp, mcpServerInfo) => {
		selectedMcp = mcp;
		setup(mcpServerInfo, mcp.catalogId);
	}}
	{selectedMcpIds}
	submitText={catalogSubmitText}
	preselectedMcp={preselected}
/>

<PageLoading show={processing} text="Launching and connecting to MCP server...">
	{#snippet longLoadContent()}
		<p class="text-sm">
			This may take a while...
			<button
				class="button-link font-semibold text-blue-500"
				onclick={() => {
					processing = false;
					if (onFinish) {
						onFinish(projectMcp, project);
					} else if (project) {
						goto(`/o/${project.id}`);
					}
				}}
			>
				Click here
			</button>
			{refProject ? 'to return to your agent.' : 'to continue to your agent.'}
		</p>
	{/snippet}
</PageLoading>

<McpInfoConfig
	bind:this={mcpInfoConfig}
	disableOutsideClick
	hideCloseButton
	manifest={projectMcp}
	{project}
	{legacyBundleId}
	prefilledConfig={projectMcpServerInfo}
	onUpdate={async (mcpServerInfo) => {
		setup(mcpServerInfo);
	}}
	submitText="Retry"
	info={selectedMcp}
>
	{#snippet leftActionContent()}
		<button
			class="button-secondary"
			onclick={async () => {
				if (onFinish) {
					onFinish(projectMcp, project);
				} else if (project) {
					await goto(`/o/${project.id}`);
				}
			}}
		>
			I'll Fix This Later
		</button>
	{/snippet}
	<p class="notification-error">
		Failed to get tools, please check your configuration and try again.
	</p>
</McpInfoConfig>

<dialog
	bind:this={projectMcpServerToolsDialog}
	class="default-dialog w-full max-w-(--breakpoint-xl) p-4 pb-0"
	class:mobile-screen-dialog={responsive.isMobile}
	use:clickOutside={() => {
		close();
	}}
>
	{#if projectMcp && project}
		{#key projectMcp.id}
			<ProjectMcpServerTools
				tools={projectMcpServerTools}
				mcpServer={projectMcp}
				{project}
				onSubmit={async () => {
					if (onFinish) {
						onFinish(projectMcp, project);
					} else if (project) {
						await goto(`/o/${project.id}`);
					}
				}}
				isNew
			/>
		{/key}
	{/if}
</dialog>

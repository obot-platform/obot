<script lang="ts">
	import {
		ChatService,
		EditorService,
		type MCP,
		type MCPServerTool,
		type Project,
		type ProjectMCP
	} from '$lib/services';
	import { createProjectMcp, type MCPServerInfo, updateProjectMcp } from '$lib/services/chat/mcp';
	import { LoaderCircle } from 'lucide-svelte';
	import McpCatalog from './McpCatalog.svelte';
	import McpInfoConfig from '$lib/components/mcp/McpInfoConfig.svelte';
	import { goto } from '$app/navigation';
	import McpServerTools from './McpServerTools.svelte';
	import { responsive } from '$lib/stores';
	import { clickOutside } from '$lib/actions/clickoutside';
	import { fade } from 'svelte/transition';
	import { getToolBundleMap } from '$lib/context/toolReferences.svelte';

	interface Props {
		mcps: MCP[];
		catalogDescription?: string;
		catalogSubmitText?: string;
		inline?: boolean;
		onFinish?: (projectMcp?: ProjectMCP, project?: Project) => void;
		project?: Project;
		selectedMcpIds?: string[];
		preselected?: string;
	}

	let {
		mcps,
		catalogDescription,
		catalogSubmitText,
		inline,
		onFinish,
		project: refProject,
		selectedMcpIds,
		preselected
	}: Props = $props();
	let project = $state(refProject);
	let projectMcp = $state<ProjectMCP>();
	let projectMcpServerInfo = $state<MCPServerInfo>();
	let projectMcpServerTools = $state<MCPServerTool[]>([]);

	let mcpCatalog = $state<ReturnType<typeof McpCatalog>>();
	let mcpInfoConfig = $state<ReturnType<typeof McpInfoConfig>>();
	let projectMcpServerToolsDialog = $state<HTMLDialogElement>();

	const toolBundleMap = getToolBundleMap();
	let legacyBundleId = $derived(
		projectMcp?.catalogID && toolBundleMap.get(projectMcp.catalogID)
			? projectMcp.catalogID
			: undefined
	);

	let processing = $state(false);

	export function open() {
		mcpCatalog?.open();
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
	{mcps}
	subtitle={catalogDescription}
	onSetupMcp={(mcpId, mcpServerInfo) => {
		setup(mcpServerInfo, mcpId);
	}}
	{selectedMcpIds}
	submitText={catalogSubmitText}
	preselectedMcp={preselected}
/>

{#if processing}
	<div
		in:fade={{ duration: 200 }}
		class="fixed top-0 left-0 z-50 flex h-svh w-svw items-center justify-center bg-black/50"
	>
		<LoaderCircle class="size-10 animate-spin" />
	</div>
{/if}

<McpInfoConfig
	bind:this={mcpInfoConfig}
	disableOutsideClick
	hideCloseButton
	manifest={projectMcp}
	{project}
	{legacyBundleId}
	prefilledConfig={projectMcpServerInfo}
	manifestType={projectMcpServerInfo?.url ? 'url' : 'command'}
	onUpdate={async (mcpServerInfo) => {
		setup(mcpServerInfo);
	}}
	submitText="Retry"
>
	{#snippet leftActionContent()}
		<button
			class="button-secondary"
			onclick={async () => {
				if (onFinish) {
					onFinish();
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
		<McpServerTools
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
	{/if}
</dialog>

<script lang="ts">
	import { closeSidebarConfig, getLayout } from '$lib/context/chatLayout.svelte';
	import type { Project, ProjectMCP } from '$lib/services';
	import { Server, X } from 'lucide-svelte';
	import McpServerInfo from '../mcp/McpServerInfo.svelte';
	import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
	import McpServerActions from './McpServerActions.svelte';
	import { getProjectMCPs, validateOauthProjectMcps } from '$lib/context/projectMcps.svelte';

	interface Props {
		mcpServer: ProjectMCP;
		project: Project;
	}

	let { mcpServer, project }: Props = $props();
	const layout = getLayout();
	const projectMcps = getProjectMCPs();
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
			<McpServerActions {mcpServer} {project} onDelete={() => closeSidebarConfig(layout)} />
			<div class="flex grow justify-end">
				<button class="icon-button" onclick={() => closeSidebarConfig(layout)}>
					<X class="size-6" />
				</button>
			</div>
		</div>
		<McpServerInfo
			catalogId={DEFAULT_MCP_CATALOG_ID}
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
			{project}
		/>
	</div>
</div>

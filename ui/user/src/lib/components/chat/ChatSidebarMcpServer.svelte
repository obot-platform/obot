<script lang="ts">
	import { closeSidebarConfig, getLayout } from '$lib/context/chatLayout.svelte';
	import type { MCPCatalogServer, Project, ProjectMCP } from '$lib/services';
	import { Server, X } from 'lucide-svelte';
	import McpServerInfo from '../mcp/McpServerInfo.svelte';
	import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
	import McpServerActions from './McpServerActions.svelte';

	interface Props {
		mcpServer: ProjectMCP;
		project: Project;
	}

	let { mcpServer, project }: Props = $props();
	const layout = getLayout();
</script>

<div class="flex h-fit w-full justify-center bg-gray-50 dark:bg-black">
	<div class="h-fit w-full px-4 py-4 md:max-w-[1200px] md:px-8">
		<div class="mb-4 flex items-center gap-2">
			{#if mcpServer.manifest.icon}
				<img
					src={mcpServer.manifest.icon}
					alt={mcpServer.manifest.name}
					class="bg-surface1 size-10 rounded-md p-1 dark:bg-gray-600"
				/>
			{:else}
				<Server class="bg-surface1 size-10 rounded-md p-1 dark:bg-gray-600" />
			{/if}
			<h1 class="text-2xl font-semibold capitalize">
				{mcpServer.manifest.name}
			</h1>
			<McpServerActions {mcpServer} {project} onDelete={() => closeSidebarConfig(layout)} />
			<div class="flex grow justify-end">
				<button class="icon-button" onclick={() => closeSidebarConfig(layout)}>
					<X class="size-6" />
				</button>
			</div>
		</div>
		<McpServerInfo catalogId={DEFAULT_MCP_CATALOG_ID} entry={mcpServer as MCPCatalogServer} />
	</div>
</div>

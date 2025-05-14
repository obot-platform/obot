<script lang="ts">
	import { closeSidebarConfig, getLayout } from '$lib/context/layout.svelte';
	import type { Assistant, AssistantTool, Project } from '$lib/services';
	import { fade } from 'svelte/transition';
	import Slack from '$lib/components/slack/Slack.svelte';
	import CustomTool from '$lib/components/edit/CustomTool.svelte';
	import ProjectInvitations from '$lib/components/edit/ProjectInvitations.svelte';
	import TemplateConfig from '$lib/components/templates/TemplateConfig.svelte';
	import { getProjectTools } from '$lib/context/projectTools.svelte';
	import ProjectMcpConfig from '$lib/components/mcp/ProjectMcpConfig.svelte';
	import { createProjectMcp, updateProjectMcp } from '$lib/services/chat/mcp';
	import { getProjectMCPs } from '$lib/context/projectMcps.svelte';
	import Discord from '$lib/components/discord/Discord.svelte';
	import McpServerTools from '$lib/components/mcp/McpServerTools.svelte';
	import ModelProviders from './ModelProviders.svelte';
	import { X } from 'lucide-svelte';

	interface Props {
		project: Project;
		currentThreadID?: string;
		assistant?: Assistant;
	}

	let { project = $bindable(), currentThreadID = $bindable() }: Props = $props();
	const layout = getLayout();

	const projectTools = getProjectTools();
	const projectMCPs = getProjectMCPs();
	let toEdit = $state<AssistantTool>();

	$effect(() => {
		if (layout.customToolId) {
			toEdit = projectTools.tools.find((t) => t.id === layout.customToolId);
		}
	});
</script>

<div class="default-scrollbar-thin relative flex w-full justify-center overflow-y-auto" in:fade>
	{#if layout.sidebarConfig === 'slack'}
		<Slack {project} inline />
	{:else if layout.sidebarConfig === 'invitations'}
		<ProjectInvitations {project} />
	{:else if layout.sidebarConfig === 'custom-mcp'}
		{#key layout.editProjectMcp?.id}
			<ProjectMcpConfig
				{project}
				projectMcp={layout.editProjectMcp}
				onCreate={async (customMcpConfig) => {
					const newProjectMcp = await createProjectMcp(customMcpConfig, project);
					projectMCPs.items.push(newProjectMcp);
					closeSidebarConfig(layout);
				}}
				onUpdate={async (customMcpConfig) => {
					if (!layout.editProjectMcp) return;
					const updatedProjectMcp = await updateProjectMcp(
						customMcpConfig,
						layout.editProjectMcp.id,
						project
					);
					projectMCPs.items = projectMCPs.items.map((mcp) =>
						mcp.id === layout.editProjectMcp!.id ? updatedProjectMcp : mcp
					);
					closeSidebarConfig(layout);
				}}
			/>
		{/key}
	{:else if layout.sidebarConfig === 'mcp-server-tools' && layout.mcpServer}
		{#key layout.mcpServer.id}
			<div class="flex w-full justify-center px-4 py-4 md:px-8">
				<div class="flex w-full flex-col gap-4 md:max-w-[1200px]">
					<McpServerTools
						{project}
						mcpServer={layout.mcpServer}
						onSubmit={() => closeSidebarConfig(layout)}
						submitText="Update"
						classes={{ actions: 'dark:bg-black' }}
					>
						{#snippet header()}
							<h2 class="flex items-center justify-between text-xl font-semibold">
								Manage Tools
								<button onclick={() => closeSidebarConfig(layout)} class="icon-button">
									<X class="size-6" />
								</button>
							</h2>
						{/snippet}
					</McpServerTools>
				</div>
			</div>
		{/key}
	{:else if layout.sidebarConfig === 'discord'}
		<Discord {project} />
	{:else if layout.sidebarConfig === 'custom-tool' && layout.customToolId && toEdit}
		{#key layout.customToolId}
			<CustomTool
				bind:tool={toEdit}
				{project}
				onSave={async (tool) => {
					projectTools.tools = projectTools.tools.map((t) => (t.id === tool.id ? tool : t));
				}}
				onDelete={async (tool) => {
					projectTools.tools = projectTools.tools.filter((t) => t.id !== tool.id);
					closeSidebarConfig(layout);
				}}
			/>
		{/key}
	{:else if layout.sidebarConfig === 'template' && layout.template}
		{#key layout.template.id}
			<TemplateConfig bind:template={layout.template} />
		{/key}
	{:else if layout.sidebarConfig === 'model-providers'}
		<ModelProviders bind:project />
	{:else}
		<div class="p-8">
			{@render underConstruction()}
		</div>
	{/if}
</div>

{#snippet underConstruction()}
	<div class="flex w-full flex-col items-center justify-center font-light">
		<img src="/user/images/under-construction.webp" alt="under construction" class="size-32" />
		<p class="text-sm font-light text-gray-500">
			This section is under construction. Please check back later.
		</p>
	</div>
{/snippet}

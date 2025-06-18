<script lang="ts">
	import { closeSidebarConfig, getLayout } from '$lib/context/layout.svelte';
	import type { Assistant, Project } from '$lib/services';
	import { fade } from 'svelte/transition';
	import Slack from '$lib/components/integrations/slack/Slack.svelte';
	import ProjectInvitations from '$lib/components/edit/ProjectInvitations.svelte';
	import TemplateConfig from '$lib/components/templates/TemplateConfig.svelte';
	import ProjectMcpConfig from '$lib/components/mcp/ProjectMcpConfig.svelte';
	import { updateProjectMcp, getKeyValuePairs } from '$lib/services/chat/mcp';
	import { getProjectMCPs } from '$lib/context/projectMcps.svelte';
	import McpServerTools from '$lib/components/mcp/McpServerTools.svelte';
	import ModelProviders from './ModelProviders.svelte';
	import ChatbotConfig from '$lib/components/edit/ChatbotConfig.svelte';
	import { X } from 'lucide-svelte';
	import Discord from './integrations/discord/Discord.svelte';
	import Webhook from './integrations/webhook/Webhook.svelte';
	import Email from './integrations/email/Email.svelte';
	import { ChatService } from '$lib/services';

	interface Props {
		project: Project;
		currentThreadID?: string;
		assistant?: Assistant;
	}

	let { project = $bindable(), currentThreadID = $bindable() }: Props = $props();
	const layout = getLayout();

	const projectMCPs = getProjectMCPs();
</script>

<div class="default-scrollbar-thin relative flex w-full justify-center overflow-y-auto" in:fade>
	{#if layout.sidebarConfig === 'slack'}
		<Slack {project} />
	{:else if layout.sidebarConfig === 'invitations'}
		<ProjectInvitations {project} />
	{:else if layout.sidebarConfig === 'custom-mcp'}
		{#key layout.editProjectMcp?.id}
			<ProjectMcpConfig
				{project}
				projectMcp={layout.editProjectMcp}
				chatbot={layout.chatbotMcpEdit}
				onCreate={async (newProjectMcp) => {
					projectMCPs.items.push(newProjectMcp);
					closeSidebarConfig(layout);
				}}
				onUpdate={async (customMcpConfig) => {
					if (!layout.editProjectMcp) return;

					if (layout.chatbotMcpEdit) {
						const keyValuePairs = getKeyValuePairs(customMcpConfig);

						await ChatService.configureProjectMCPEnvHeaders(
							project.assistantID,
							project.id,
							layout.editProjectMcp.id,
							keyValuePairs
						);

						projectMCPs.items = projectMCPs.items.map((mcp) => {
							if (mcp.id !== layout.editProjectMcp!.id) return mcp;
							return {
								...mcp,
								env: customMcpConfig.env,
								headers: customMcpConfig.headers
							};
						});
					} else {
						const updatedProjectMcp = await updateProjectMcp(
							customMcpConfig,
							layout.editProjectMcp.id,
							project
						);
						projectMCPs.items = projectMCPs.items.map((mcp) =>
							mcp.id === layout.editProjectMcp!.id ? updatedProjectMcp : mcp
						);
					}

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
	{:else if layout.sidebarConfig === 'webhook'}
		<Webhook {project} />
	{:else if layout.sidebarConfig === 'email'}
		<Email {project} />
	{:else if layout.sidebarConfig === 'chatbot'}
		<ChatbotConfig {project} />
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

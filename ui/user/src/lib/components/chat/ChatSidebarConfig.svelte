<script lang="ts">
	import { closeSidebarConfig, getLayout } from '$lib/context/chatLayout.svelte';
	import type { Assistant, Project } from '$lib/services';
	import { fade } from 'svelte/transition';
	import Slack from '$lib/components/integrations/slack/Slack.svelte';
	import ProjectInvitations from '$lib/components/edit/ProjectInvitations.svelte';
	import TemplateConfig from '$lib/components/templates/TemplateConfig.svelte';
	import ProjectMcpServerTools from '$lib/components/mcp/ProjectMcpServerTools.svelte';
	import ModelProviders from '../ModelProviders.svelte';
	import ChatbotConfig from '$lib/components/edit/ChatbotConfig.svelte';
	import { X } from 'lucide-svelte';
	import Discord from '../integrations/discord/Discord.svelte';
	import Webhook from '../integrations/webhook/Webhook.svelte';
	import Email from '../integrations/email/Email.svelte';

	interface Props {
		project: Project;
		currentThreadID?: string;
		assistant?: Assistant;
	}

	let { project = $bindable(), currentThreadID = $bindable() }: Props = $props();
	const layout = getLayout();
</script>

<div class="default-scrollbar-thin relative flex w-full justify-center overflow-y-auto" in:fade>
	{#if layout.sidebarConfig === 'slack'}
		<Slack {project} />
	{:else if layout.sidebarConfig === 'invitations'}
		<ProjectInvitations {project} />
	{:else if layout.sidebarConfig === 'mcp-server-tools' && layout.mcpServer}
		{#key layout.mcpServer.id}
			<div class="flex w-full justify-center px-4 py-4 md:px-8">
				<div class="flex w-full flex-col gap-4 md:max-w-[1200px]">
					<ProjectMcpServerTools
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
					</ProjectMcpServerTools>
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

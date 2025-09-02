<script lang="ts">
	import { getLayout } from '$lib/context/chatLayout.svelte';
	import type { Assistant, Project } from '$lib/services';
	import { fade } from 'svelte/transition';
	import Slack from '$lib/components/integrations/slack/Slack.svelte';
	import ProjectInvitations from '$lib/components/edit/ProjectInvitations.svelte';
	import TemplateConfig from '$lib/components/templates/TemplateConfig.svelte';
	import ModelProviders from '../ModelProviders.svelte';
	import Discord from '../integrations/discord/Discord.svelte';
	import Webhook from '../integrations/webhook/Webhook.svelte';
	import Email from '../integrations/email/Email.svelte';
	import ChatSidebarMcpServer from './ChatSidebarMcpServer.svelte';
	import ProjectConfiguration from '../edit/ProjectConfiguration.svelte';

	interface Props {
		project: Project;
		currentThreadID?: string;
		assistant?: Assistant;
	}

	let { project = $bindable(), currentThreadID = $bindable() }: Props = $props();
	const layout = getLayout();
</script>

<div
	class="default-scrollbar-thin relative flex w-full justify-center overflow-y-auto bg-gray-50 dark:bg-black"
	in:fade
>
	{#if layout.sidebarConfig === 'project-configuration'}
		{#key project.id}
			<ProjectConfiguration bind:project />
		{/key}
	{:else if layout.sidebarConfig === 'slack'}
		<Slack {project} />
	{:else if layout.sidebarConfig === 'invitations'}
		<ProjectInvitations {project} />
	{:else if (layout.sidebarConfig === 'mcp-server-tools' && layout.mcpServer) || (layout.sidebarConfig === 'mcp-server' && layout.mcpServer)}
		{#key layout.mcpServer.id}
			<ChatSidebarMcpServer
				mcpServer={layout.mcpServer}
				{project}
				view={layout.sidebarConfig === 'mcp-server-tools' ? 'tools' : 'overview'}
			/>
		{/key}
	{:else if layout.sidebarConfig === 'discord'}
		<Discord {project} />
	{:else if layout.sidebarConfig === 'webhook'}
		<Webhook {project} />
	{:else if layout.sidebarConfig === 'email'}
		<Email {project} />
	{:else if layout.sidebarConfig === 'template'}
		<TemplateConfig assistantID={project.assistantID} projectID={project.id} />
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

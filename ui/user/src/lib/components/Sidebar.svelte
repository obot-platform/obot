<script lang="ts">
	import { type Assistant, type Project } from '$lib/services';
	import { KeyRound, SidebarClose, Brain, Settings } from 'lucide-svelte';
	import Threads from '$lib/components/sidebar/Threads.svelte';
	import { hasTool } from '$lib/tools';
	import Credentials from '$lib/components/navbar/Credentials.svelte';
	import Tasks from '$lib/components/sidebar/Tasks.svelte';
	import { getLayout, openSidebarConfig, type Layout } from '$lib/context/layout.svelte';
	import Projects from './navbar/Projects.svelte';
	import Logo from './navbar/Logo.svelte';
	import Tables from '$lib/components/sidebar/Tables.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { getProjectTools } from '$lib/context/projectTools.svelte';
	import MemoriesDialog from '$lib/components/MemoriesDialog.svelte';
	import McpServers from './sidebar/McpServers.svelte';
	// import Instructions from './edit/Instructions.svelte';
	import Knowledge from './edit/Knowledge.svelte';
	import Sites from './edit/Sites.svelte';
	import CustomTools from './edit/CustomTools.svelte';
	import Files from './edit/Files.svelte';
	import Slack from './slack/Slack.svelte';
	import { version } from '$lib/stores';
	import { browser } from '$app/environment';
	import ChatBot from './edit/ChatBot.svelte';
	import Introduction from './edit/Introduction.svelte';
	import Members from './edit/Members.svelte';
	import Interfaces from './edit/Interfaces.svelte';

	interface Props {
		project: Project;
		currentThreadID?: string;
		assistant?: Assistant;
	}

	let { project = $bindable(), currentThreadID = $bindable(), assistant }: Props = $props();
	let credentials = $state<ReturnType<typeof Credentials>>();
	let memories = $state<ReturnType<typeof MemoriesDialog>>();
	let projectsOpen = $state(false);
	const layout = getLayout();
	const projectTools = getProjectTools();

	const sidebarConfigValues = [
		{
			label: 'System Prompt',
			value: 'system-prompt'
		},
		{
			label: 'Introduction & Starter Messages',
			value: 'introduction'
		},
		{
			label: 'Create Agent Template',
			value: 'template'
		}
	];
</script>

<div class="bg-surface1 relative flex size-full flex-col">
	<div class="flex h-16 flex-shrink-0 items-center justify-between px-3">
		<div
			class="flex items-center transition-all duration-300"
			class:w-full={projectsOpen}
			class:w-[calc(100%-42px)]={!projectsOpen}
		>
			<span class="shrink-0"><Logo class="ml-0" /></span>
			<Projects
				{project}
				onOpenChange={(open) => (projectsOpen = open)}
				disabled={layout.projectEditorOpen}
				classes={{
					tooltip:
						'md:min-w-[250px] md:w-1/6 md:-translate-x-14 -translate-x-1 border-t-[1px] border-surface3 bg-surface2 shadow-inner max-h-[calc(100vh-66px)] overflow-y-auto default-scrollbar-thin pb-0'
				}}
				showDelete
			/>
		</div>
		<button
			class:opacity-0={projectsOpen}
			class:w-0={projectsOpen}
			class:!min-w-0={projectsOpen}
			class:!p-0={projectsOpen}
			class="icon-button overflow-hidden transition-all duration-300"
			onclick={() => (layout.sidebarOpen = false)}
			use:tooltip={'Close Sidebar'}
		>
			<SidebarClose class="icon-default" />
		</button>
	</div>

	<div class="default-scrollbar-thin flex w-full grow flex-col gap-4 px-3 pb-5">
		{#if project.editor}
			<McpServers {project} />
			<Tasks {project} bind:currentThreadID />
			<Threads {project} bind:currentThreadID />
			{#if hasTool(projectTools.tools, 'database')}
				<Tables {project} />
			{/if}
			<Knowledge {project} />
			<!-- {#if assistant?.websiteKnowledge?.siteTool}
				<Sites {project} />
			{/if}
			{#if version.current.dockerSupported}
				<CustomTools {project} />
			{/if} -->
			<Files {project} compact />
			<Members {project} />
			<!--
			<Instructions bind:project />
			<Introduction bind:project />
			<Slack {project} />
			<ChatBot {project} />
			 -->
			<Interfaces {project} />
			{#each sidebarConfigValues as config}
				<div class="flex flex-col gap-2">
					<div class="mb-1 flex items-center justify-between">
						<p class="grow text-sm font-semibold">{config.label}</p>
						<button
							class="icon-button"
							onclick={() => openSidebarConfig(layout, config.value as Layout['sidebarConfig'])}
							use:tooltip={`Open ${config.label}`}
						>
							<Settings class="size-5" />
						</button>
					</div>
				</div>
			{/each}
		{:else}
			<Threads {project} bind:currentThreadID />
		{/if}
	</div>

	<div class="flex gap-1 px-3 py-2">
		<button class="icon-button" onclick={() => credentials?.show()} use:tooltip={'Credentials'}>
			<KeyRound class="icon-default" />
		</button>
		<Credentials bind:this={credentials} {project} {currentThreadID} />

		{#if hasTool(projectTools.tools, 'memory')}
			<button
				class="icon-button"
				onclick={() => memories?.show()}
				use:tooltip={'Memories'}
				data-memories-btn
			>
				<Brain class="icon-default" />
			</button>
		{/if}

		<MemoriesDialog bind:this={memories} {project} />
	</div>
</div>

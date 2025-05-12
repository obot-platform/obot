<script lang="ts">
	import { ChatService, type Project } from '$lib/services';
	import { MessageCirclePlus, SidebarClose } from 'lucide-svelte';
	import { hasTool } from '$lib/tools';
	import { closeAll, getLayout } from '$lib/context/layout.svelte';
	import Tasks from '$lib/components/edit/Tasks.svelte';
	import General from '$lib/components/edit/General.svelte';
	import McpServers from '$lib/components/edit/McpServers.svelte';
	import Knowledge from '$lib/components/edit/Knowledge.svelte';
	import Files from '$lib/components/edit/Files.svelte';
	import Sharing from '$lib/components/edit/Sharing.svelte';
	import Interfaces from '$lib/components/edit/Interfaces.svelte';
	import CustomTools from '$lib/components/edit/CustomTools.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { getProjectTools } from '$lib/context/projectTools.svelte';
	import Threads from '$lib/components/sidebar/Threads.svelte';
	import Tables from '$lib/components/sidebar/Tables.svelte';
	import ModelProviders from '$lib/components/sidebar/ModelProviders.svelte';
	import SystemPrompt from '$lib/components/edit/SystemPrompt.svelte';
	import Introduction from '$lib/components/edit/Introduction.svelte';
	import { responsive, version } from '$lib/stores';
	import Logo from '$lib/components/navbar/Logo.svelte';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { getHelperMode, HELPER_TEXTS } from '$lib/context/helperMode.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import Memories from '$lib/components/edit/Memories.svelte';
	import { scrollFocus } from '$lib/actions/scrollFocus.svelte';
	import BuiltInCapabilities from '$lib/components/edit/BuiltInCapabilities.svelte';

	interface Props {
		project: Project;
		currentThreadID?: string;
		shared?: boolean;
	}

	let { project = $bindable(), currentThreadID = $bindable(), shared }: Props = $props();
	const layout = getLayout();
	const projectTools = getProjectTools();
	const helperMode = getHelperMode();

	async function createNewThread() {
		const thread = await ChatService.createThread(project.assistantID, project.id);
		const found = layout.threads?.find((t) => t.id === thread.id);
		if (!found) {
			layout.threads?.splice(0, 0, thread);
		}

		closeAll(layout);
		currentThreadID = thread.id;
	}
</script>

<div class="bg-surface1 dark:bg-surface2 relative flex size-full flex-col">
	<div class="flex h-16 w-full flex-shrink-0 items-center px-3">
		<button
			onclick={() => {
				layout.sidebarOpen = false;
			}}
			use:tooltip={'Close Sidebar'}
		>
			<Logo class="ml-0" />
		</button>
		<div class="flex grow"></div>
		{#if !shared}
			<button
				class="icon-button p-0.5"
				use:tooltip={'Start New Thread'}
				onclick={() => createNewThread()}
			>
				<MessageCirclePlus class="size-6" />
			</button>
		{/if}
		{#if responsive.isMobile}
			{@render closeSidebar()}
		{/if}
	</div>
	<div class="default-scrollbar-thin flex w-full grow flex-col" use:scrollFocus>
		{#if project.editor && !shared}
			<Threads {project} bind:currentThreadID editor />
			<Tasks {project} bind:currentThreadID />
			<McpServers {project} />
			<BuiltInCapabilities bind:project />
			{#if hasTool(projectTools.tools, 'memory')}
				<Memories {project} />
			{/if}

			{#if hasTool(projectTools.tools, 'database')}
				<Tables {project} editor />
			{/if}
			<div class="mt-auto flex flex-col">
				<CollapsePane
					classes={{
						header: 'pl-3 border-y border-surface2 dark:border-surface3 py-2',
						content: 'p-0 bg-transparent dark:bg-transparent shadow-none',
						headerText: 'text-sm font-medium'
					}}
					header="Configuration"
					helpText={HELPER_TEXTS.configuration}
					iconSize={5}
				>
					<General bind:project />
					<SystemPrompt bind:project />
					<Introduction bind:project />
					<Knowledge {project} />
					<Files {project} classes={{ list: 'text-sm flex flex-col gap-2' }} />
					<ModelProviders {project} />
					{#if version.current.dockerSupported}
						<CustomTools {project} />
					{/if}
					<Interfaces />
					<Sharing {project} />
				</CollapsePane>
			</div>
		{:else}
			<Threads {project} bind:currentThreadID />
			{#if hasTool(projectTools.tools, 'memory')}
				<Memories {project} />
			{/if}
		{/if}
	</div>

	<div class="flex items-center justify-between px-3 py-2">
		<div class="flex items-center gap-1">
			<Toggle
				label="Toggle Help"
				labelInline
				checked={helperMode.isEnabled}
				onChange={() => (helperMode.isEnabled = !helperMode.isEnabled)}
			/>
		</div>

		{#if !responsive.isMobile}
			{@render closeSidebar()}
		{/if}
	</div>
</div>

{#snippet closeSidebar()}
	<button class="icon-button" onclick={() => (layout.sidebarOpen = false)}>
		<SidebarClose class="size-6" />
	</button>
{/snippet}

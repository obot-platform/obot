<script lang="ts">
	import { type Project } from '$lib/services';
	import { Plus, Settings, SidebarClose } from 'lucide-svelte';
	import { hasTool } from '$lib/tools';
	import { closeAll, getLayout, openConfigureProject } from '$lib/context/chatLayout.svelte';
	import Tasks from '$lib/components/edit/Tasks.svelte';
	import McpServers from '$lib/components/edit/McpServers.svelte';

	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { getProjectTools } from '$lib/context/projectTools.svelte';
	import Threads from '$lib/components/chat/sidebar/Threads.svelte';

	import { responsive } from '$lib/stores';
	import Memories from '$lib/components/edit/Memories.svelte';
	import { scrollFocus } from '$lib/actions/scrollFocus.svelte';
	import Projects from '../navbar/Projects.svelte';
	import BetaLogo from '../navbar/BetaLogo.svelte';

	interface Props {
		project: Project;
		currentThreadID?: string;
		shared?: boolean;
		onCreateProject?: () => void;
	}

	let {
		project = $bindable(),
		currentThreadID = $bindable(),
		shared,
		onCreateProject
	}: Props = $props();
	const layout = getLayout();
	const projectTools = getProjectTools();
</script>

<div class="border-surface2 dark:bg-gray-990 relative flex size-full flex-col border-r bg-white">
	<div class="flex h-16 w-full flex-shrink-0 items-center px-3">
		<BetaLogo />
		{#if responsive.isMobile}
			{@render closeSidebar()}
		{/if}
	</div>
	<div class="default-scrollbar-thin flex w-full grow flex-col gap-2 px-3 pt-8" use:scrollFocus>
		<div class="flex w-[calc(100%+0.25em)] -translate-x-1 items-center justify-between">
			<Projects {project} />
			<div class="flex items-center">
				<button
					class="icon-button flex-shrink-0"
					onclick={() => {
						openConfigureProject(layout);
					}}
					use:tooltip={'Configure Current Project'}
				>
					<Settings class="size-5" />
				</button>
				<button
					class="icon-button flex-shrink-0"
					onclick={() => {
						closeAll(layout);
						onCreateProject?.();
					}}
					use:tooltip={'Create New Project'}
				>
					<Plus class="size-5" />
				</button>
			</div>
		</div>
		{#if project.editor && !shared}
			<Threads {project} bind:currentThreadID />
			<Tasks {project} bind:currentThreadID />
			<McpServers {project} />
			{#if hasTool(projectTools.tools, 'memory')}
				<Memories {project} />
			{/if}
		{:else}
			<Threads {project} bind:currentThreadID />
			<McpServers {project} chatbot={true} />
			{#if hasTool(projectTools.tools, 'memory')}
				<Memories {project} />
			{/if}
		{/if}
	</div>

	<div class="flex items-center justify-end px-3 py-2">
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

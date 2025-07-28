<script lang="ts">
	import { type Project } from '$lib/services';
	import { SidebarClose } from 'lucide-svelte';
	import { hasTool } from '$lib/tools';
	import { getLayout } from '$lib/context/chatLayout.svelte';
	import Tasks from '$lib/components/edit/Tasks.svelte';
	import McpServers from '$lib/components/edit/McpServers.svelte';

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
	<div class="flex h-16 w-full flex-shrink-0 items-center px-2">
		<BetaLogo />
		{#if responsive.isMobile}
			{@render closeSidebar()}
		{/if}
	</div>
	<div class="default-scrollbar-thin flex w-full grow flex-col gap-2" use:scrollFocus>
		<Projects {project} {onCreateProject} />
		<div class="flex flex-col gap-8 px-4">
			{#if project.editor && !shared}
				<Threads {project} bind:currentThreadID />
				{#if hasTool(projectTools.tools, 'memory')}
					<Memories {project} />
				{/if}
				<Tasks {project} bind:currentThreadID />
				<McpServers {project} />
			{:else}
				<Threads {project} bind:currentThreadID />
				{#if hasTool(projectTools.tools, 'memory')}
					<Memories {project} />
				{/if}
				<McpServers {project} chatbot={true} />
			{/if}
		</div>
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

<script lang="ts">
	import { type Project } from '$lib/services';
	import { Settings, SidebarClose } from 'lucide-svelte';
	import { getLayout } from '$lib/context/chatLayout.svelte';
	import Tasks from '$lib/components/edit/Tasks.svelte';
	import McpServers from '$lib/components/edit/McpServers.svelte';

	import Threads from '$lib/components/chat/sidebar/Threads.svelte';

	import { responsive } from '$lib/stores';
	import { scrollFocus } from '$lib/actions/scrollFocus.svelte';
	import Projects from '../navbar/Projects.svelte';
	import BetaLogo from '../navbar/BetaLogo.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';

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
</script>

<div class="border-surface2 dark:bg-gray-990 relative flex size-full flex-col border-r bg-white">
	<div class="flex h-16 w-full flex-shrink-0 items-center justify-between px-2 md:justify-start">
		<BetaLogo chat />
		{#if responsive.isMobile}
			{@render closeSidebar()}
		{/if}
	</div>
	<div class="default-scrollbar-thin flex w-full grow flex-col gap-2" use:scrollFocus>
		<Projects {project} {onCreateProject} />
		<div class="flex flex-col gap-8 px-4">
			{#if project.editor && !shared}
				<Threads {project} bind:currentThreadID />
				<Tasks {project} bind:currentThreadID />
				<McpServers {project} />
			{:else}
				<Threads {project} bind:currentThreadID />
				<McpServers {project} />
			{/if}
		</div>
	</div>

	<div class="flex w-full items-center justify-between gap-2 px-2 py-2">
		<button
			class="icon-button"
			onclick={() => (layout.sidebarConfig = 'project-configuration')}
			use:tooltip={'Configure Project'}
		>
			<Settings class="size-6 text-gray-500" />
		</button>
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

<script lang="ts">
	import { type AssistantTool, type Project } from '$lib/services';
	import { KeyRound, SidebarClose } from 'lucide-svelte';
	import Threads from '$lib/components/sidebar/Threads.svelte';
	import Clone from '$lib/components/navbar/Clone.svelte';
	import { hasTool } from '$lib/tools';
	import Term from '$lib/components/navbar/Term.svelte';
	import Credentials from '$lib/components/navbar/Credentials.svelte';
	import Tasks from '$lib/components/sidebar/Tasks.svelte';
	import { getLayout } from '$lib/context/layout.svelte';
	import Projects from './navbar/Projects.svelte';
	import Logo from './navbar/Logo.svelte';
	import Settings from './navbar/Settings.svelte';

	interface Props {
		project: Project;
		currentThreadID?: string;
		tools: AssistantTool[];
	}

	let { project, currentThreadID = $bindable(), tools }: Props = $props();
	let credentials = $state<ReturnType<typeof Credentials>>();
	let projectsOpen = $state(false);
	const layout = getLayout();
</script>

<div class="relative flex size-full flex-col bg-surface1">
	<div class="flex h-[76px] items-center justify-between p-3">
		<div
			class="flex h-[52px] items-center transition-all duration-300"
			class:w-full={projectsOpen}
			class:w-[calc(100%-42px)]={!projectsOpen}
		>
			<span class="flex-shrink-0"><Logo class="ml-0" /></span>
			<Projects {project} onOpenChange={(open) => (projectsOpen = open)} />
		</div>
		<button
			class:opacity-0={projectsOpen}
			class:w-0={projectsOpen}
			class:!min-w-0={projectsOpen}
			class:!p-0={projectsOpen}
			class="icon-button overflow-hidden transition-all duration-300"
			onclick={() => (layout.sidebarOpen = false)}
		>
			<SidebarClose class="icon-default" />
		</button>
	</div>

	<div class="default-scrollbar-thin flex w-full grow flex-col gap-2 px-3 pb-5">
		<Threads {project} bind:currentThreadID />
		<Tasks {project} bind:currentThreadID />
	</div>

	<div class="flex gap-1 px-3 pb-2">
		{#if (layout.sidebarOpen && !layout.projectEditorOpen)}
			<Settings />
		{/if}
		{#if hasTool(tools, 'shell')}
			<Term />
		{/if}
		<button class="icon-button" onclick={() => credentials?.show()}>
			<KeyRound class="icon-default" />
		</button>
		<Credentials bind:this={credentials} {project} {tools} />
		{#if !project.editor}
			<Clone {project} />
		{/if}
	</div>
</div>

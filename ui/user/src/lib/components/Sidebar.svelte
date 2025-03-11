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

	interface Props {
		project: Project;
		currentThreadID?: string;
		tools: AssistantTool[];
	}

	let { project, currentThreadID = $bindable(), tools }: Props = $props();
	let credentials = $state<ReturnType<typeof Credentials>>();
	const layout = getLayout();
</script>

<div class="relative flex size-full flex-col gap-3 bg-surface1">
	<div class="flex justify-between h-[76px] items-center p-3">
		<div class="flex items-center w-[calc(100%-42px)] h-[52px]">
			<span class="flex-shrink-0"><Logo /></span>
			<Projects {project} />
		</div>
		<button class="icon-button" onclick={() => (layout.sidebarOpen = false)}>
			<SidebarClose class="icon-default" />
		</button>
	</div>

	<div class="default-scrollbar-thin flex w-full grow flex-col gap-2 p-3">
		<Threads {project} bind:currentThreadID />
		<Tasks {project} />
	</div>

	<div class="flex justify-end gap-1 px-3 pb-2">
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

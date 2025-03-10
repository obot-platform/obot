<script lang="ts">
	import { type AssistantTool, type Project } from '$lib/services';
	import { KeyRound, SidebarClose } from 'lucide-svelte';
	import AssistantIcon from '$lib/icons/AssistantIcon.svelte';
	import Threads from '$lib/components/sidebar/Threads.svelte';
	import Clone from '$lib/components/navbar/Clone.svelte';
	import { hasTool } from '$lib/tools';
	import Term from '$lib/components/navbar/Term.svelte';
	import Credentials from '$lib/components/navbar/Credentials.svelte';
	import Tasks from '$lib/components/sidebar/Tasks.svelte';
	import { getLayout } from '$lib/context/layout.svelte';

	interface Props {
		project: Project;
		currentThreadID?: string;
		tools: AssistantTool[];
	}

	let { project, currentThreadID = $bindable(), tools }: Props = $props();
	let credentials = $state<ReturnType<typeof Credentials>>();
	const layout = getLayout();
</script>

<div class="relative flex size-full flex-col gap-3 rounded-tl-3xl bg-surface1">
	<button class="icon-button absolute right-1 top-1" onclick={() => (layout.sidebarOpen = false)}>
		<SidebarClose class="icon-default" />
	</button>

	<div class="flex flex-col gap-2 p-3 pb-0">
		<div class="flex items-center gap-2 rounded-lg">
			<AssistantIcon {project} class="h-5 w-5" />
			<span class="text-xl font-semibold text-on-background">{project.name || 'Untitled'}</span>
			<div class="grow"></div>
		</div>
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

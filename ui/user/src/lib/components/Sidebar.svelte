<script lang="ts">
	import { type AssistantTool, type Project } from '$lib/services';
	import { KeyRound, SidebarClose } from 'lucide-svelte';
	import Threads from '$lib/components/sidebar/Threads.svelte';
	import Clone from '$lib/components/navbar/Clone.svelte';
	import { hasTool } from '$lib/tools';
	import Credentials from '$lib/components/navbar/Credentials.svelte';
	import Tasks from '$lib/components/sidebar/Tasks.svelte';
	import { getLayout } from '$lib/context/layout.svelte';
	import Projects from './navbar/Projects.svelte';
	import Logo from './navbar/Logo.svelte';
	import Tables from '$lib/components/sidebar/Tables.svelte';
	import { popover } from '$lib/actions';

	interface Props {
		project: Project;
		currentThreadID?: string;
		tools: AssistantTool[];
	}

	let { project, currentThreadID = $bindable(), tools }: Props = $props();
	let credentials = $state<ReturnType<typeof Credentials>>();
	let projectsOpen = $state(false);
	const layout = getLayout();

	let credentialsTT = popover({ hover: true, placement: 'right' });
</script>

<div class="bg-surface1 relative flex size-full flex-col">
	<div class="flex h-[76px] items-center justify-between p-3">
		<div
			class="flex h-[52px] items-center transition-all duration-300"
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
						'-translate-x-1 md:-translate-x-14 border-t-[1px] border-surface3 bg-surface2 shadow-inner max-h-[calc(100vh-66px)] overflow-y-auto default-scrollbar-thin'
				}}
			/>
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
		{#if hasTool(tools, 'database')}
			<Tables {project} />
		{/if}
	</div>

	<div class="flex gap-1 px-3 py-2">
		<p use:credentialsTT.tooltip class="tooltip">Credentials</p>

		<button class="icon-button" onclick={() => credentials?.show()} use:credentialsTT.ref>
			<KeyRound class="icon-default" />
		</button>

		<Credentials bind:this={credentials} {project} {tools} />
		{#if !project.editor}
			<Clone {project} />
		{/if}
	</div>
</div>

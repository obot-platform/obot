<script lang="ts">
	import { type Project } from '$lib/services';
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
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { getProjectTools } from '$lib/context/projectTools.svelte';

	interface Props {
		project: Project;
		currentThreadID?: string;
	}

	let { project, currentThreadID = $bindable() }: Props = $props();
	let credentials = $state<ReturnType<typeof Credentials>>();
	let projectsOpen = $state(false);
	const layout = getLayout();
	const projectTools = getProjectTools();
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
						'md:min-w-[250px] md:w-1/6 md:-translate-x-14 -translate-x-1 border-t-[1px] border-surface3 bg-surface2 shadow-inner max-h-[calc(100vh-66px)] overflow-y-auto default-scrollbar-thin'
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
			use:tooltip={'Close Sidebar'}
		>
			<SidebarClose class="icon-default" />
		</button>
	</div>

	<div class="default-scrollbar-thin flex w-full grow flex-col gap-2 px-3 pb-5">
		<Threads {project} bind:currentThreadID />
		<Tasks {project} bind:currentThreadID />
		{#if hasTool(projectTools.tools, 'database')}
			<Tables {project} />
		{/if}
	</div>

	<div class="flex gap-1 px-3 py-2">
		<button class="icon-button" onclick={() => credentials?.show()} use:tooltip={'Credentials'}>
			<KeyRound class="icon-default" />
		</button>

		<Credentials bind:this={credentials} {project} />
		{#if !project.editor}
			<Clone {project} />
		{/if}
	</div>
</div>

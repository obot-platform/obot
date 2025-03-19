<script lang="ts">
	import Editor from '$lib/components/Editors.svelte';
	import Navbar from '$lib/components/Navbar.svelte';
	import Notifications from '$lib/components/Notifications.svelte';
	import Sidebar from '$lib/components/Sidebar.svelte';
	import Task from '$lib/components/tasks/Task.svelte';
	import Thread from '$lib/components/Thread.svelte';
	import { getLayout } from '$lib/context/layout.svelte';
	import { type AssistantTool, ChatService, type Project, type Version } from '$lib/services';
	import type { EditorItem } from '$lib/services/editor/index.svelte';
	import { responsive, term } from '$lib/stores';
	import { SidebarOpen } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fade, slide } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';
	import Logo from './navbar/Logo.svelte';

	interface Props {
		project: Project;
		items?: EditorItem[];
		tools?: AssistantTool[];
		currentThreadID?: string;
	}

	let {
		project,
		tools = [],
		currentThreadID = $bindable(),
		items = $bindable([])
	}: Props = $props();
	let layout = getLayout();
	let editorVisible = $derived(layout.fileEditorOpen || term.open);
	let version = $state<Version>({});

	onMount(async () => {
		if (tools.length === 0) {
			tools = (await ChatService.listTools(project.assistantID, project.id)).items;
		}
		if (!version) {
			version = await ChatService.getVersion();
		}
	});
</script>

<div class="colors-background relative flex h-full flex-col overflow-hidden">
	<div
		class="border-surface1 relative flex h-full"
		class:border={layout.sidebarOpen && !layout.fileEditorOpen}
	>
		{#if layout.sidebarOpen && !layout.fileEditorOpen}
			<div class="w-screen min-w-screen md:w-1/6 md:min-w-[250px]" transition:slide={{ axis: 'x' }}>
				<Sidebar {project} bind:currentThreadID {tools} />
			</div>
		{/if}

		<main
			id="main-content"
			class="flex max-w-full grow flex-col overflow-hidden"
			class:hidden={layout.sidebarOpen && responsive.isMobile}
		>
			<div class="h-[76px] w-full">
				<Navbar showEditorButton={!layout.projectEditorOpen} {project}>
					{#if !layout.sidebarOpen || layout.fileEditorOpen}
						<Logo />
						<button
							class="icon-button"
							in:fade={{ delay: 400 }}
							onclick={() => {
								layout.sidebarOpen = true;
								layout.fileEditorOpen = false;
							}}
						>
							<SidebarOpen class="icon-default" />
						</button>
					{/if}
				</Navbar>
			</div>

			<div class="flex h-[calc(100%-76px)] max-w-full grow">
				{#if !responsive.isMobile || (responsive.isMobile && !editorVisible)}
					{#if layout.editTaskID && layout.tasks}
						{#each layout.tasks as task, i}
							{#if task.id === layout.editTaskID}
								{#key layout.editTaskID}
									<Task
										{project}
										bind:task={layout.tasks[i]}
										onDelete={() => {
											layout.editTaskID = undefined;
											layout.tasks?.splice(i, 1);
										}}
									/>
								{/key}
							{/if}
						{/each}
					{:else}
						<div id="main-input" class="flex h-full max-w-full flex-1 justify-center">
							<Thread
								bind:id={currentThreadID}
								{project}
								{version}
								{tools}
								isTaskRun={!!currentThreadID &&
									!!layout.taskRuns?.some((run) => run.id === currentThreadID)}
							/>
						</div>
					{/if}
				{/if}
				<div
					class={twMerge(
						'border-surface2 absolute right-0 float-right w-full translate-x-full transform border-4 border-r-0 pt-2 transition-transform duration-300 md:mb-8 md:w-3/5 md:max-w-[calc(100%-320px)] md:min-w-[320px] md:rounded-l-3xl md:ps-5 md:pt-5',
						editorVisible && 'relative w-full translate-x-0',
						!editorVisible && 'w-0!'
					)}
				>
					<Editor {project} {currentThreadID} />
				</div>
			</div>

			<Notifications />
		</main>
	</div>
</div>

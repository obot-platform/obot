<script lang="ts">
	import Editor from '$lib/components/Editors.svelte';
	import Navbar from '$lib/components/Navbar.svelte';
	import Sidebar from '$lib/components/Sidebar.svelte';
	import Task from '$lib/components/tasks/Task.svelte';
	import Thread from '$lib/components/Thread.svelte';
	import { ChatService, type Project } from '$lib/services';
	import type { EditorItem } from '$lib/services/editor/index.svelte';
	import { responsive } from '$lib/stores';
	import { closeAll, getLayout } from '$lib/context/layout.svelte';
	import { GripVertical, Plus, SidebarOpen } from 'lucide-svelte';
	import { fade, slide } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';
	import Logo from './navbar/Logo.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { columnResize } from '$lib/actions/resize';

	interface Props {
		project: Project;
		items?: EditorItem[];
		currentThreadID?: string;
	}

	let { project = $bindable(), currentThreadID = $bindable() }: Props = $props();
	let layout = getLayout();
	let editor: HTMLDivElement | undefined = $state();

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

<div class="colors-background relative flex h-full flex-col overflow-hidden">
	<div
		class="border-surface1 relative flex h-full"
		class:border={layout.sidebarOpen && !layout.fileEditorOpen}
	>
		{#if layout.sidebarOpen && !layout.fileEditorOpen}
			<div class="w-screen min-w-screen md:w-1/6 md:min-w-[250px]" transition:slide={{ axis: 'x' }}>
				<Sidebar {project} bind:currentThreadID />
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
							use:tooltip={'Open Sidebar'}
						>
							<SidebarOpen class="icon-default" />
						</button>
						<button
							class="icon-button"
							in:fade={{ delay: 400 }}
							use:tooltip={'Start New Thread'}
							onclick={() => createNewThread()}
						>
							<Plus class="icon-default" />
						</button>
					{/if}
				</Navbar>
			</div>

			<div class="relative flex h-[calc(100%-76px)] max-w-full grow">
				{#if !responsive.isMobile || (responsive.isMobile && !layout.fileEditorOpen)}
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
					{:else if layout.displayTaskRun}
						<Task
							{project}
							task={{
								...layout.displayTaskRun.task,
								id: layout.displayTaskRun.taskID
							}}
							runID={layout.displayTaskRun.id}
						/>
					{:else}
						<Thread bind:id={currentThreadID} bind:project />
					{/if}
				{/if}

				<div
					bind:this={editor}
					class={twMerge(
						'border-surface2 absolute right-0 z-30 float-right flex w-full translate-x-full transform border-4 border-r-0 transition-transform duration-300 md:w-3/5 md:max-w-[calc(100%-320px)] md:min-w-[320px] md:rounded-l-3xl',
						layout.fileEditorOpen && 'relative w-full translate-x-0',
						!layout.fileEditorOpen && 'w-0'
					)}
				>
					{#if editor && layout.fileEditorOpen}
						<div
							use:columnResize={{ column: editor, direction: 'right' }}
							class="relative h-full w-8 cursor-grab"
							transition:slide={{ axis: 'x' }}
						>
							<div
								class="text-on-surface1 absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2"
							>
								<GripVertical class="text-surface3 size-3" />
							</div>
						</div>
					{/if}
					<Editor {project} {currentThreadID} />
				</div>
			</div>
		</main>
	</div>
</div>

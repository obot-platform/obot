<script lang="ts">
	import Editor from '$lib/components/Editors.svelte';
	import Navbar from '$lib/components/Navbar.svelte';
	import Sidebar from '$lib/components/chat/ChatSidebar.svelte';
	import Task from '$lib/components/tasks/Task.svelte';
	import Thread from '$lib/components/Thread.svelte';
	import { ChatService, type Project } from '$lib/services';
	import type { EditorItem } from '$lib/services/editor/index.svelte';
	import { darkMode, responsive } from '$lib/stores';
	import { closeAll, getLayout } from '$lib/context/chatLayout.svelte';
	import { GripVertical, MessageCirclePlus, SidebarOpen } from 'lucide-svelte';
	import { fade, slide } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { columnResize } from '$lib/actions/resize';
	import { X } from 'lucide-svelte';
	import type { Assistant, CreateProjectForm } from '$lib/services';
	import { clickOutside } from '$lib/actions/clickoutside';
	import SidebarConfig from './chat/ChatSidebarConfig.svelte';
	import { onMount, onDestroy } from 'svelte';
	import { browser } from '$app/environment';
	import McpServerOauths from './chat/McpServerOauths.svelte';

	interface Props {
		assistant?: Assistant;
		project: Project;
		items?: EditorItem[];
		currentThreadID?: string;
		shared?: boolean;
	}

	let { project = $bindable(), currentThreadID = $bindable(), assistant, shared }: Props = $props();
	let layout = getLayout();
	let editor: HTMLDivElement | undefined = $state();
	let createProject = $state<CreateProjectForm>();

	let shortcutsDialog: HTMLDialogElement;
	let nav = $state<HTMLDivElement>();

	async function createNewThread() {
		const thread = await ChatService.createThread(project.assistantID, project.id);
		const found = layout.threads?.find((t) => t.id === thread.id);
		if (!found) {
			layout.threads?.splice(0, 0, thread);
		}

		closeAll(layout);
		currentThreadID = thread.id;
	}

	function handleKeydown(event: KeyboardEvent) {
		// Ctrl + E for edit mode
		if (event.ctrlKey && event.key === 'e') {
			event.preventDefault();
			layout.projectEditorOpen = !layout.projectEditorOpen;
		}

		// Ctrl + T for thread panel
		if (event.ctrlKey && event.key === 't') {
			event.preventDefault();
			layout.sidebarOpen = !layout.sidebarOpen;
			layout.fileEditorOpen = false;
		}

		// Ctrl + H for keyboard shortcuts help
		if (event.ctrlKey && event.key === 'h') {
			event.preventDefault();
			shortcutsDialog?.showModal();
		}
	}

	function initProjectForm(): CreateProjectForm {
		return {
			name: '',
			description: '',
			icons: undefined,
			prompt: ''
		};
	}

	onMount(() => {
		if (browser) {
			window.addEventListener('keydown', handleKeydown);
		}
	});

	onDestroy(() => {
		if (browser) {
			window.removeEventListener('keydown', handleKeydown);
		}
	});
</script>

<div class="colors-background relative flex h-full flex-col overflow-hidden">
	<div
		class="border-surface1 relative flex h-full"
		class:border={layout.sidebarOpen && !layout.fileEditorOpen}
	>
		{#if layout.sidebarOpen && !layout.fileEditorOpen}
			<div
				class="bg-surface1 w-screen min-w-screen flex-shrink-0 md:w-1/6 md:min-w-[250px]"
				transition:slide={{ axis: 'x' }}
				bind:this={nav}
			>
				<Sidebar
					bind:project
					bind:currentThreadID
					onCreateProject={() => (createProject = initProjectForm())}
				/>
			</div>
			{#if !responsive.isMobile}
				<div
					role="none"
					class="relative -ml-3 h-full w-3 cursor-col-resize"
					use:columnResize={{ column: nav }}
				></div>
			{/if}
		{/if}

		<main
			id="main-content"
			class="flex max-w-full grow flex-col overflow-hidden"
			class:hidden={layout.sidebarOpen && responsive.isMobile}
		>
			<div class="w-full">
				<Navbar>
					{#snippet leftContent()}
						{#if !layout.sidebarOpen || layout.fileEditorOpen}
							{@render logo()}
							<button
								class="icon-button ml-2 p-0.5"
								in:fade={{ delay: 350, duration: 0 }}
								use:tooltip={'Start New Thread'}
								onclick={() => createNewThread()}
							>
								<MessageCirclePlus class="size-6" />
							</button>
						{/if}
						{#if !layout.sidebarOpen && responsive.isMobile}
							{@render openSidebar()}
						{/if}
					{/snippet}
				</Navbar>
			</div>

			{#if !layout.sidebarOpen && !responsive.isMobile}
				<div class="absolute bottom-2 left-2 z-30" in:fade={{ delay: 300 }}>
					{@render openSidebar()}
				</div>
			{/if}

			<div
				class="relative flex h-[calc(100%-76px)] max-w-full grow"
				class:pl-12={!layout.sidebarOpen && !responsive.isMobile && !layout.sidebarConfig}
			>
				{#if !responsive.isMobile || (responsive.isMobile && !layout.fileEditorOpen)}
					{#if layout.editTaskID && layout.tasks}
						{#each layout.tasks as task, i (task.id)}
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
						{#key layout.displayTaskRun.id}
							<Task
								{project}
								task={{
									...layout.displayTaskRun.task,
									id: layout.displayTaskRun.taskID
								}}
								runID={layout.displayTaskRun.id}
							/>
						{/key}
					{:else if layout.sidebarConfig}
						<SidebarConfig bind:project bind:currentThreadID {assistant} />
					{:else}
						<Thread bind:id={currentThreadID} bind:project {shared} bind:createProject />
					{/if}
				{/if}

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
				<div
					bind:this={editor}
					class={twMerge(
						'border-surface2 absolute right-0 z-30 float-right flex w-full flex-shrink-0 translate-x-full transform border-4 border-r-0 transition-transform duration-300 md:w-3/5 md:max-w-[calc(100%-320px)] md:min-w-[320px] md:rounded-l-3xl',
						layout.fileEditorOpen && 'relative w-full translate-x-0',
						!layout.fileEditorOpen && 'w-0'
					)}
				>
					<Editor {project} {currentThreadID} />
				</div>
			</div>

			<dialog
				bind:this={shortcutsDialog}
				class="default-dialog"
				use:clickOutside={() => shortcutsDialog?.close()}
			>
				<div class="p-6">
					<button class="absolute top-0 right-0 p-3" onclick={() => shortcutsDialog?.close()}>
						<X class="icon-default" />
					</button>
					<h3 class="mb-4 text-lg font-semibold">Keyboard Shortcuts</h3>
					<div class="space-y-4">
						<div class="grid grid-cols-2 gap-2">
							<div class="font-medium">Ctrl + E</div>
							<div>Toggle Edit Mode</div>

							<div class="font-medium">Ctrl + T</div>
							<div>Toggle Thread Panel</div>

							<div class="font-medium">Ctrl + H</div>
							<div>Show Keyboard Shortcuts</div>
						</div>
					</div>
				</div>
			</dialog>
		</main>
	</div>
</div>

<McpServerOauths assistantId={assistant?.id || ''} projectId={project.id} />

{#snippet openSidebar()}
	<button class="icon-button" onclick={() => (layout.sidebarOpen = true)}>
		<SidebarOpen class="size-6" />
	</button>
{/snippet}

{#snippet logo()}
	<div class="flex flex-shrink-0 items-center">
		{#if darkMode.isDark}
			<img src="/user/images/obot-logo-blue-white-text.svg" class="h-12" alt="Obot logo" />
		{:else}
			<img src="/user/images/obot-logo-blue-black-text.svg" class="h-12" alt="Obot logo" />
		{/if}
		<div class="ml-1.5 -translate-y-1">
			<span
				class="rounded-full border-2 border-blue-400 px-1.5 py-[1px] text-[10px] font-bold text-blue-400 dark:border-blue-400 dark:text-blue-400"
			>
				BETA
			</span>
		</div>
	</div>
{/snippet}

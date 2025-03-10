<script lang="ts">
	import { term } from '$lib/stores';
	import Navbar from '$lib/components/Navbar.svelte';
	import Editor from '$lib/components/Editors.svelte';
	import { type AssistantTool, ChatService, type Project, type Version } from '$lib/services';
	import Notifications from '$lib/components/Notifications.svelte';
	import Thread from '$lib/components/Thread.svelte';
	import { columnResize } from '$lib/actions/resize';
	import { onMount } from 'svelte';
	import { getLayout } from '$lib/context/layout.svelte';
	import type { EditorItem } from '$lib/services/editor/index.svelte';
	import Sidebar from '$lib/components/Sidebar.svelte';
	import Task from '$lib/components/tasks/Task.svelte';
	import { slide, fade } from 'svelte/transition';
	import { SidebarOpen } from 'lucide-svelte';

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

	let mainInput = $state<HTMLDivElement>();

	onMount(async () => {
		if (tools.length === 0) {
			tools = (await ChatService.listTools(project.assistantID, project.id)).items;
		}
		if (!version) {
			version = await ChatService.getVersion();
		}
	});
</script>

<div class="colors-background relative flex h-full flex-col">
	<div class="h-[76px]">
		<Navbar {project} />
	</div>

	<div
		class="flex h-[calc(100%-76px)] rounded-t-3xl border-surface1"
		class:border={layout.sidebarOpen && !layout.fileEditorOpen}
	>
		{#if layout.sidebarOpen && !layout.fileEditorOpen}
			<div class="w-1/6 min-w-[250px]" transition:slide={{ axis: 'x' }}>
				<Sidebar {project} bind:currentThreadID {tools} />
			</div>
		{:else if !layout.fileEditorOpen}
			<button
				class="icon-button absolute bottom-0 left-0 z-50 m-2"
				in:fade={{ delay: 400 }}
				onclick={() => (layout.sidebarOpen = true)}
			>
				<SidebarOpen class="icon-default" />
			</button>
		{/if}

		<main id="main-content" class="flex grow">
			{#if layout.editTaskID && layout.tasks}
				{#each layout.tasks as task, i}
					{#if task.id === layout.editTaskID}
						{#key layout.editTaskID}
							<Task {project} bind:task={layout.tasks[i]} />
						{/key}
					{/if}
				{/each}
			{:else}
				<div
					bind:this={mainInput}
					id="main-input"
					class="flex h-full {editorVisible ? 'w-2/5' : 'grow'}"
				>
					<Thread bind:id={currentThreadID} {project} {version} {tools} />
				</div>
			{/if}

			{#if editorVisible && mainInput}
				<div class="w-4 translate-x-4 cursor-col-resize" use:columnResize={mainInput}></div>
				<div
					class="w-3/5 grow rounded-tl-3xl border-4 border-b-0 border-r-0 border-surface2 p-5 pb-0 pr-0 transition-all"
				>
					<Editor {project} {currentThreadID} />
				</div>
			{/if}
		</main>

		<Notifications />
	</div>
</div>

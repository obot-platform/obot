<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import { getLayout, openTask } from '$lib/context/layout.svelte';
	import { ChatService, type Project, type Task } from '$lib/services';
	import { Plus, Trash2 } from 'lucide-svelte/icons';
	import { onMount } from 'svelte';
	import TaskItem from '../shared/task/TaskItem.svelte';
	import { responsive } from '$lib/stores';
	import { tooltip } from '$lib/actions/tooltip.svelte';

	interface Props {
		project: Project;
		currentThreadID?: string;
	}

	let { currentThreadID = $bindable(), project }: Props = $props();
	const layout = getLayout();

	async function deleteTask() {
		if (!taskToDelete?.id) {
			return;
		}
		await ChatService.deleteTask(project.assistantID, project.id, taskToDelete.id);
		if (layout.editTaskID === taskToDelete.id) {
			openTask(layout, undefined);
		}
		taskToDelete = undefined;
		await reload();
	}

	async function newTask() {
		const task = await ChatService.createTask(project.assistantID, project.id, {
			id: '',
			name: 'New Task',
			steps: []
		});
		if (!layout.tasks) {
			layout.tasks = [];
		}
		layout.tasks.splice(0, 0, task);
		if (responsive.isMobile) {
			layout.sidebarOpen = false;
		}
		openTask(layout, task.id);
	}

	async function reload() {
		layout.tasks = (await ChatService.listTasks(project.assistantID, project.id)).items;
	}

	onMount(() => {
		reload();
	});

	let taskToDelete = $state<Task>();
</script>

<div class="flex w-full flex-col">
	<div class="mb-1 flex items-center gap-1">
		<p class="grow text-sm font-semibold">Tasks</p>
		<button class="icon-button" onclick={() => newTask()} use:tooltip={'Create New Task'}>
			<Plus class="icon-default" />
		</button>
	</div>
	{#if !layout.tasks || layout.tasks.length === 0}
		<p class="text-gray p-6 text-center text-sm dark:text-gray-300">No tasks</p>
	{:else}
		<ul>
			{#each layout.tasks as task, i (task.id)}
				<TaskItem
					{task}
					{project}
					taskRuns={layout.taskRuns?.filter((run) => run.taskID === task.id) ?? []}
					expanded={i < 5}
					bind:currentThreadID
				>
					{#snippet taskActions()}
						<button
							class="p-0 pr-2 opacity-0 transition-opacity duration-200 group-hover:opacity-100"
							onclick={() => (taskToDelete = task)}
							use:tooltip={'Delete Task'}
						>
							<Trash2 class="size-4" />
						</button>
					{/snippet}
				</TaskItem>
			{/each}
		</ul>
	{/if}
</div>

<Confirm
	show={taskToDelete !== undefined}
	msg={`Are you sure you want to delete ${taskToDelete?.name}?`}
	onsuccess={deleteTask}
	oncancel={() => (taskToDelete = undefined)}
/>

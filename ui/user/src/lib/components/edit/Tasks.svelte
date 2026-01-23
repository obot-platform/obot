<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import { createNewTask, getLayout, openTask, openTaskRun } from '$lib/context/chatLayout.svelte';
	import { ChatService, type Project, type Task } from '$lib/services';
	import { Plus } from 'lucide-svelte/icons';
	import { onMount } from 'svelte';
	import { responsive } from '$lib/stores';
	import TaskItem from '$lib/components/edit/TaskItem.svelte';
	import Input from '$lib/components/tasks/Input.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { fade } from 'svelte/transition';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';

	interface Props {
		project: Project;
		currentThreadID?: string;
	}

	let { currentThreadID = $bindable(), project }: Props = $props();
	const layout = getLayout();
	let inputDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let waitingTask = $state<Task>();
	let waitingTaskInput = $state('');

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
		if (responsive.isMobile) {
			layout.sidebarOpen = false;
		}
		createNewTask(layout);
	}

	async function reload() {
		layout.tasks = (await ChatService.listTasks(project.assistantID, project.id)).items;
	}

	async function runTask(task?: Task) {
		if (!task) return;

		if (task.onDemand && !waitingTaskInput) {
			waitingTask = task;
			inputDialog?.open();
		} else {
			const response = await ChatService.runTask(project.assistantID, project.id, task.id, {
				input: waitingTaskInput ?? ''
			});

			openTaskRun(
				layout,
				await ChatService.getTaskRun(project.assistantID, project.id, task.id, response.id)
			);

			if (responsive.isMobile) {
				// need to close sidebar to see the task run
				layout.sidebarOpen = false;
			}

			// clear waiting task
			waitingTaskInput = '';
			waitingTask = undefined;
		}
	}

	onMount(() => {
		reload();
	});

	let taskToDelete = $state<Task>();
</script>

<div class="flex flex-col text-xs">
	<div class="flex items-center justify-between">
		<p class="text-md grow font-medium">Tasks</p>
		<button
			class="hover:text-on-background text-on-surface1 p-2 transition-colors duration-200"
			onclick={() => newTask()}
			use:tooltip={'Start New Task'}
		>
			<Plus class="size-5" />
		</button>
	</div>
	{#if layout.tasks && layout.tasks.length > 0}
		<ul class="flex flex-col" transition:fade>
			{#each layout.tasks as task, i (task.id)}
				<TaskItem
					{task}
					{project}
					taskRuns={layout.taskRuns?.filter((run) => run.taskID === task.id) ?? []}
					expanded={i < 5}
					bind:currentThreadID
					classes={{
						taskItemAction: 'pr-3'
					}}
				></TaskItem>
			{/each}
		</ul>
	{/if}
</div>

<ResponsiveDialog bind:this={inputDialog} title="Run Task" class="max-w-full md:min-w-md">
	<div class="mt-4 flex w-full md:mt-0">
		<Input bind:input={waitingTaskInput} task={waitingTask} />
	</div>
	<div class="flex grow"></div>
	<div class="mt-4 flex w-full flex-col justify-between gap-4 md:flex-row md:justify-end">
		<button
			class="button-primary w-full md:w-fit"
			onclick={() => {
				runTask(waitingTask);
				inputDialog?.close();
			}}>Run</button
		>
	</div>
</ResponsiveDialog>

<Confirm
	show={taskToDelete !== undefined}
	msg={`Delete ${taskToDelete?.name}?`}
	onsuccess={deleteTask}
	oncancel={() => (taskToDelete = undefined)}
/>

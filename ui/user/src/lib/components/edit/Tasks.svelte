<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import { getLayout, openTask, openTaskRun } from '$lib/context/chatLayout.svelte';
	import { ChatService, type Project, type Task } from '$lib/services';
	import { ChevronRight, Play, Plus, Trash2, X } from 'lucide-svelte/icons';
	import { onMount } from 'svelte';
	import { responsive } from '$lib/stores';
	import TaskItem from '$lib/components/edit/TaskItem.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Input from '$lib/components/tasks/Input.svelte';
	import { clickOutside } from '$lib/actions/clickoutside';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { HELPER_TEXTS } from '$lib/context/helperMode.svelte';

	interface Props {
		project: Project;
		currentThreadID?: string;
	}

	let { currentThreadID = $bindable(), project }: Props = $props();
	const layout = getLayout();
	let inputDialog = $state<HTMLDialogElement>();
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

	function isTaskFromIntegration(task: Task) {
		return (
			task.id === project.workflowNamesFromIntegration?.slackWorkflowName ||
			task.id === project.workflowNamesFromIntegration?.discordWorkflowName ||
			task.id === project.workflowNamesFromIntegration?.emailWorkflowName ||
			task.id === project.workflowNamesFromIntegration?.webhookWorkflowName
		);
	}
	async function runTask(task?: Task) {
		if (!task) return;

		if ((task.onDemand || task.email || task.webhook) && !waitingTaskInput) {
			waitingTask = task;
			inputDialog?.showModal();
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

<CollapsePane
	classes={{ header: 'pl-3 py-2', content: 'p-2' }}
	iconSize={5}
	header="Tasks"
	helpText={HELPER_TEXTS.tasks}
	open={(layout.tasks?.length ?? 0) > 0}
>
	<div class="flex flex-col gap-4">
		{#if layout.tasks && layout.tasks.length > 0}
			<ul class="flex flex-col">
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
					>
						{#snippet taskActions()}
							{#if !isTaskFromIntegration(task)}
								<DotDotDot
									class="p-2 pr-2.5 transition-opacity duration-200 group-hover:opacity-100 md:opacity-0"
								>
									<div class="default-dialog flex min-w-max flex-col p-2">
										<button class="menu-button" onclick={() => runTask(task)}>
											<Play class="size-4" /> Run Task
										</button>
										<button class="menu-button" onclick={() => (taskToDelete = task)}>
											<Trash2 class="size-4" /> Delete
										</button>
									</div>
								</DotDotDot>
							{/if}
						{/snippet}
					</TaskItem>
				{/each}
			</ul>
		{/if}
		<div class="flex justify-end">
			<button class="button flex items-center gap-1 text-xs" onclick={() => newTask()}>
				<Plus class="size-4" /> New Task
			</button>
		</div>
	</div>
</CollapsePane>

<dialog
	bind:this={inputDialog}
	use:clickOutside={() => inputDialog?.close()}
	class="max-w-full md:min-w-md"
	class:p-4={!responsive.isMobile}
	class:mobile-screen-dialog={responsive.isMobile}
>
	<div class="flex h-full w-full flex-col justify-between gap-4">
		<h3 class="default-dialog-title" class:default-dialog-mobile-title={responsive.isMobile}>
			Run Task
			<button
				class:mobile-header-button={responsive.isMobile}
				onclick={() => inputDialog?.close()}
				class="icon-button"
			>
				{#if responsive.isMobile}
					<ChevronRight class="size-6" />
				{:else}
					<X class="size-5" />
				{/if}
			</button>
		</h3>
		<div class="flex w-full grow">
			<Input bind:input={waitingTaskInput} task={waitingTask} />
		</div>
		<div class="mt-4 flex w-full flex-col justify-between gap-4 md:flex-row md:justify-end">
			<button
				class="button-primary w-full md:w-fit"
				onclick={() => {
					runTask(waitingTask);
					inputDialog?.close();
				}}>Run</button
			>
		</div>
	</div>
</dialog>

<Confirm
	show={taskToDelete !== undefined}
	msg={`Are you sure you want to delete ${taskToDelete?.name}?`}
	onsuccess={deleteTask}
	oncancel={() => (taskToDelete = undefined)}
/>

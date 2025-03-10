<script lang="ts">
	import { Plus, Trash } from 'lucide-svelte/icons';
	import { ChatService, type Project, type Task } from '$lib/services';
	import Confirm from '$lib/components/Confirm.svelte';
	import { CheckSquare, Play } from 'lucide-svelte';
	import { getLayout } from '$lib/context/layout.svelte';
	import { onMount } from 'svelte';
	import { overflowToolTip } from '$lib/actions/overflow';

	interface Props {
		project: Project;
	}

	let { project }: Props = $props();
	const layout = getLayout();

	async function deleteTask() {
		if (!taskToDelete?.id) {
			return;
		}
		await ChatService.deleteTask(project.assistantID, project.id, taskToDelete.id);
		if (layout.editTaskID === taskToDelete.id) {
			layout.editTaskID = undefined;
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
		layout.editTaskID = task.id;
		if (!layout.tasks) {
			layout.tasks = [];
		}
		layout.tasks.splice(0, 0, task);
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
	<div class="flex items-center">
		<CheckSquare class="me-2 h-5 w-5 text-gray" />
		<h2 class="grow text-lg">Tasks</h2>
		<button class="icon-button" onclick={() => newTask()}>
			<Plus class="icon-default" />
		</button>
	</div>
	{#if !layout.tasks || layout.tasks.length === 0}
		<p class="p-6 text-center text-sm text-gray dark:text-gray-300">No tasks</p>
	{:else}
		<ul class="space-y-4 py-6">
			{#each layout.tasks as task}
				<li class="flex flex-col">
					<div class="flex items-center">
						<button
							use:overflowToolTip
							class="flex w-[50%] flex-1 items-center"
							onclick={async () => {
								layout.editTaskID = task.id;
							}}
						>
							{task.name ?? ''}
						</button>
						<button
							onclick={async () => {
								await ChatService.runTask(project.assistantID, project.id, task.id);
							}}
						>
							<Play class="size-5 text-gray-400" />
						</button>
						<button
							onclick={() => {
								taskToDelete = task;
							}}
						>
							<Trash class="size-5 text-gray-400" />
						</button>
					</div>
				</li>
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

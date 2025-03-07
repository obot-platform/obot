<script lang="ts">
	import { Plus, Trash } from 'lucide-svelte/icons';
	import { ChatService, EditorService, type Project, type Task } from '$lib/services';
	import Confirm from '$lib/components/Confirm.svelte';
	import { CheckSquare } from 'lucide-svelte';
	import Menu from '$lib/components/navbar/Menu.svelte';
	import { getLayout } from '$lib/context/layout.svelte';
	import Truncate from '$lib/components/shared/tooltip/Truncate.svelte';

	interface Props {
		project: Project;
	}

	let { project }: Props = $props();
	let tasks = $state<Task[]>([]);
	const layout = getLayout();

	async function deleteTask() {
		if (!taskToDelete?.id) {
			return;
		}
		await ChatService.deleteTask(project.assistantID, project.id, taskToDelete.id);
		EditorService.remove(layout.items, taskToDelete.id);
		menu?.toggle(false);
		taskToDelete = undefined;
	}

	async function newTask() {
		const task = await ChatService.createTask(project.assistantID, project.id, {
			id: '',
			name: 'New Task',
			steps: []
		});
		await EditorService.load(layout.items, project, task.id);
		layout.fileEditorOpen = true;
		menu?.toggle(false);
	}

	async function reload() {
		tasks = (await ChatService.listTasks(project.assistantID, project.id)).items;
	}

	let taskToDelete = $state<Task | undefined>();
	let menu = $state<ReturnType<typeof Menu>>();
</script>

<Menu bind:this={menu} title="Tasks" description="Helpful automations" onLoad={() => reload()}>
	{#snippet icon()}
		<CheckSquare class="h-5 w-5" />
	{/snippet}
	{#snippet body()}
		{#if tasks.length === 0}
			<p class="p-6 text-center text-sm text-gray dark:text-gray-300">No tasks</p>
		{:else}
			<ul class="space-y-4 py-6 text-sm">
				{#each tasks as task}
					<li class="group">
						<div class="flex">
							<button
								class="flex flex-1 items-center"
								onclick={async () => {
									await EditorService.load(layout.items, project, task.id);
									layout.fileEditorOpen = true;
									menu?.toggle(false);
								}}
							>
								<CheckSquare class="size-5 min-w-fit" />
								<Truncate class="ms-2 group-hover:underline" text={task.name ?? ''} />
							</button>
							<button
								class="invisible group-hover:visible"
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
		<div class="flex justify-end">
			<button onclick={newTask} class="button -mb-3 -mr-3 mt-3 flex items-center justify-end gap-2">
				Add Task
				<Plus class="ms-1 h-5 w-5" />
			</button>
		</div>
	{/snippet}
</Menu>

<Confirm
	show={taskToDelete !== undefined}
	msg={`Are you sure you want to delete ${taskToDelete?.name}?`}
	onsuccess={deleteTask}
	oncancel={() => (taskToDelete = undefined)}
/>

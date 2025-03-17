<script lang="ts">
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { getLayout } from '$lib/context/layout.svelte.js';
	import { ChatService, type Project, type Task } from '$lib/services';
	import { Edit, Plus } from 'lucide-svelte/icons';
	import TaskEditor from '$lib/components/tasks/Task.svelte';
	import { X } from 'lucide-svelte';

	interface Props {
		project: Project;
	}

	let { project }: Props = $props();
	let editIndex = $state<number>();
	let editorDialog: HTMLDialogElement;
	const layout = getLayout();

	function shareTask(task: Task, checked: boolean) {
		if (checked) {
			if (!project.sharedTasks?.find((id) => id === task.id)) {
				if (!project.sharedTasks) {
					project.sharedTasks = [];
				}
				project.sharedTasks.push(task.id);
			}
		} else {
			project.sharedTasks = project.sharedTasks?.filter((id) => id !== task.id);
		}
	}

	async function newTask() {
		const newTask = await ChatService.createTask(project.assistantID, project.id, {
			id: '',
			name: 'New Task',
			steps: []
		});
		if (!layout.tasks) {
			layout.tasks = [];
		}
		layout.tasks.push(newTask);
		if (!project.sharedTasks) {
			project.sharedTasks = [];
		}
		project.sharedTasks.push(newTask.id);
	}

	async function edit(index: number) {
		editIndex = index;
		editorDialog?.showModal();
	}

	async function closeEdit() {
		editIndex = undefined;
		editorDialog?.close();
	}
</script>

<CollapsePane header="Tasks">
	<p class="mb-4 text-sm">The following tasks will be shared with users of this Obot.</p>
	{#each layout.tasks ?? [] as task, i (task.id)}
		<div class="ml-4 flex items-center gap-2">
			<input
				checked={project.sharedTasks?.includes(task.id)}
				type="checkbox"
				onchange={(e) => {
					if (e.target instanceof HTMLInputElement) {
						shareTask(task, e.target.checked);
					}
				}}
			/>
			<span class="mr-2">{task.name}</span>
			<button class="icon-button" onclick={() => edit(i)}>
				<Edit class="icon-default" />
			</button>
		</div>
	{/each}
	<button class="button flex items-center gap-1 self-end text-sm" onclick={() => newTask()}>
		<Plus class="size-4" />
		New Task
	</button>
</CollapsePane>

<dialog bind:this={editorDialog} class="relative h-full w-full md:w-4/5">
	<button class="icon-button absolute right-2 top-2 z-10" onclick={() => closeEdit()}>
		<X class="icon-default" />
	</button>
	{#if editIndex !== undefined && layout.tasks}
		<TaskEditor {project} bind:task={layout.tasks[editIndex]} />
	{/if}
</dialog>

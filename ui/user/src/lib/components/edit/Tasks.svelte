<script lang="ts">
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { getLayout, openTask } from '$lib/context/layout.svelte.js';
	import { ChatService, type Project, type Task } from '$lib/services';
	import { Edit, Plus } from 'lucide-svelte/icons';

	interface Props {
		project: Project;
	}

	let { project }: Props = $props();
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
		openTask(layout, newTask.id);
	}

	async function edit(editTaskIndex: number) {
		openTask(layout, layout.tasks?.[editTaskIndex]?.id);
	}
</script>

<CollapsePane header="Tasks">
	<div class="flex w-full flex-col gap-4">
		<p class="text-gray text-sm">The following tasks will be shared with users of this Obot.</p>
		<div class="flex flex-col">
			{#each layout.tasks ?? [] as task, i (task.id)}
				<div class="flex items-center justify-between gap-2">
					<div class="flex items-center gap-2">
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
					</div>
					<button class="icon-button" onclick={() => edit(i)}>
						<Edit class="icon-default" />
					</button>
				</div>
			{/each}
			{#if layout.tasks?.length ?? 0 === 0}
				<p class="text-gray pt-6 pb-4 text-center text-sm font-light">No tasks found.</p>
			{/if}
		</div>
		<button class="button flex items-center gap-1 self-end text-sm" onclick={() => newTask()}>
			<Plus class="size-4" />
			New Task
		</button>
	</div>
</CollapsePane>

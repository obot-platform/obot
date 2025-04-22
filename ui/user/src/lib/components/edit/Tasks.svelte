<script lang="ts">
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { getLayout, openTask } from '$lib/context/layout.svelte.js';
	import { ChatService, type Project, type Task } from '$lib/services';
	import { Plus, Route, RouteOff } from 'lucide-svelte/icons';
	import { onMount } from 'svelte';
	import TaskItem from '../shared/task/TaskItem.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';

	interface Props {
		project: Project;
		currentThreadID?: string;
	}

	let { project, currentThreadID = $bindable() }: Props = $props();
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

	onMount(async () => {
		layout.tasks = (await ChatService.listTasks(project.assistantID, project.id)).items;
	});
</script>

<CollapsePane header="Tasks">
	<div class="flex w-full flex-col gap-4">
		<p class="text-gray text-sm">The following tasks will be shared with users of this agent.</p>
		<div class="flex flex-col">
			{#if layout.tasks}
				<ul>
					{#each layout.tasks as task, i (task.id)}
						<TaskItem
							{task}
							{project}
							taskRuns={layout.taskRuns?.filter((run) => run.taskID === task.id) ?? []}
							expanded={i < 5}
							classes={{
								title: 'text-sm'
							}}
							bind:currentThreadID
						>
							{#snippet taskActions()}
								<button
									class="icon-button-small hover:bg-surface2 mr-2"
									onclick={() => shareTask(task, !project.sharedTasks?.includes(task.id))}
									use:tooltip={project.sharedTasks?.includes(task.id)
										? 'This task is being shared with other users.'
										: 'This task is private and only visible to you.'}
								>
									{#if project.sharedTasks?.includes(task.id)}
										<Route class="size-4" />
									{:else}
										<RouteOff class="text-gray size-4" />
									{/if}
								</button>
							{/snippet}
						</TaskItem>
					{/each}
				</ul>
			{/if}
			{#if (layout.tasks?.length ?? 0) === 0}
				<p class="text-gray pt-6 pb-4 text-center text-sm font-light">No tasks found.</p>
			{/if}
		</div>
		<button class="button flex items-center gap-1 self-end text-sm" onclick={() => newTask()}>
			<Plus class="size-4" />
			New Task
		</button>
	</div>
</CollapsePane>

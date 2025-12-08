<script lang="ts">
	import {
		DraggableList,
		DraggableItem,
		DraggableHandle
	} from '$lib/components/primitives/draggable';
	import WorkflowTask from './WorkflowTask.svelte';

	interface Task {
		id: string;
		name: string;
		description: string;
		content: string;
	}

	interface Props {
		tasks: Task[];
		onVariableAddition?: (variable: string) => void;
		onVariableDeletion?: (variable: string) => void;
		onDelete?: (task: Task) => void;
	}

	let { tasks = $bindable(), onVariableAddition, onVariableDeletion, onDelete }: Props = $props();
</script>

<DraggableList
	as="ol"
	class="gap-6"
	gap={12}
	order={tasks.map((t) => t.id)}
	onChange={(items) => {
		tasks = items as Task[];
	}}
>
	{#each tasks as task, index (task.id)}
		<div class="workflow-task">
			<DraggableItem
				id={task.id}
				data={task}
				class="hover:border-primary hover:outline-primary bg-background dark:bg-surface1 dark:border-surface3 rounded-lg border p-0 outline-1 outline-transparent transition-all"
			>
				<div class="group relative flex h-full w-full items-start gap-3">
					<DraggableHandle class="absolute top-0 left-0 h-full w-full opacity-0" />
					<div class="flex min-w-0 flex-1 flex-col gap-1 p-6">
						<WorkflowTask
							bind:task={tasks[index]}
							{onVariableDeletion}
							{onVariableAddition}
							{onDelete}
						/>
					</div>
				</div>
			</DraggableItem>
		</div>
	{/each}
</DraggableList>

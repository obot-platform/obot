<script lang="ts">
	import {
		DraggableList,
		DraggableItem,
		DraggableHandle
	} from '$lib/components/primitives/draggable';
	import { Plus } from 'lucide-svelte';
	import WorkflowTask from './WorkflowTask.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';

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
	class="gap-6 pl-20"
	gap={12}
	order={tasks.map((t) => t.id)}
	onChange={(items) => {
		tasks = items as Task[];
	}}
>
	{#each tasks as task, index (task.id)}
		<div class="workflow-task w-full -translate-x-16">
			<DraggableItem hidePointerBorder id={task.id} data={task}>
				<div class="group relative flex w-full items-start gap-2">
					<div
						class="flex flex-shrink-0 items-center opacity-0 transition-opacity duration-200 group-hover:opacity-100"
					>
						{@render action(index)}
						<DraggableHandle class="hover:bg-surface3 size-8 rounded-full p-2" />
					</div>
					<div
						class="bg-background dark:bg-surface1 dark:border-surface3 flex w-[calc(100%-5rem)] grow rounded-lg border p-0 transition-all"
					>
						<div class="flex min-w-0 flex-1 flex-col gap-1 p-6">
							<WorkflowTask
								bind:task={tasks[index]}
								{onVariableDeletion}
								{onVariableAddition}
								{onDelete}
							/>
						</div>
					</div>
				</div>
			</DraggableItem>
		</div>
	{/each}
</DraggableList>

{#snippet action(index: number)}
	<button
		class="button-icon min-h-fit min-w-fit p-2"
		onclick={async () => {
			tasks.splice(index + 1, 0, {
				id: (tasks.length + 1).toString(),
				name: '',
				description: '',
				content: ''
			});
		}}
		use:tooltip={'Add task'}
	>
		<Plus class="size-4" />
	</button>
{/snippet}

<script lang="ts">
	import { ChatService, type Project, type Task } from '$lib/services';
	import { Trash2 } from 'lucide-svelte/icons';
	import { onDestroy, onMount } from 'svelte';
	import Trigger from '$lib/components/tasks/Trigger.svelte';
	import Steps from '$lib/components/tasks/Steps.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import { newSaveMonitor } from '$lib/save.js';

	interface Props {
		task: Task;
		project: Project;
		onChanged?: (task: Task) => void | Promise<void>;
		onDelete?: () => void | Promise<void>;
	}

	let { task = $bindable(), onChanged, project, onDelete }: Props = $props();

	const saver = newSaveMonitor(
		() => task,
		async (t: Task) => {
			return await ChatService.saveTask(project.assistantID, project.id, t);
		},
		(t) => {
			task = t;
			onChanged?.(t);
		}
	);

	$effect(() => {
		if (task.id && task.steps.length === 0) {
			task.steps.push({
				id: 'si1' + Math.random().toString(36).substring(6)
			});
		}
	});

	let toDelete: boolean = $state(false);

	async function deleteTask() {
		toDelete = false;
		await ChatService.deleteTask(project.assistantID, project.id, task.id);
		onDelete?.();
	}

	onDestroy(() => {
		saver.stop();
	});

	onMount(async () => {
		task = await ChatService.getTask(project.assistantID, project.id, task.id);
		saver.start();
	});
</script>

<div class="flex w-full justify-center overflow-y-auto scrollbar-none">
	<!-- div in div is needed for the scrollbar to work so that space outside the max-width is still scrollable -->
	<div
		role="none"
		onkeydown={(e) => e.stopPropagation()}
		class="relative flex w-full max-w-[1200px] flex-col gap-4 rounded-s-3xl"
	>
		<div class="flex w-full justify-between gap-8 px-8 py-5">
			<div class="flex grow flex-col gap-1 border-l-4 border-blue pl-4">
				<strong class="text-xs text-blue">TASK</strong>

				<input class="ghost-input text-2xl font-semibold" bind:value={task.name} />

				<input
					class="ghost-input"
					bind:value={task.description}
					placeholder="Enter description..."
				/>
			</div>
			<div class="flex items-center gap-2">
				<button class="button-destructive !p-4" onclick={() => (toDelete = true)}>
					<Trash2 class="size-4" />
				</button>
			</div>
		</div>

		<div class="flex grow flex-col gap-8 px-6">
			<Trigger bind:task />

			<Steps
				bind:task
				{project}
				save={async () => {
					await saver.save();
				}}
			/>

			<div class="grow place-content-end self-end text-gray-300">id: {task.id}</div>
		</div>
	</div>

	<Confirm
		show={toDelete}
		msg={`Are you sure you want to delete this task`}
		onsuccess={deleteTask}
		oncancel={() => (toDelete = false)}
	/>
</div>

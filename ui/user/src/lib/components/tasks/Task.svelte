<script lang="ts">
	import { ChatService, type Project, type Task } from '$lib/services';
	import { Trash } from 'lucide-svelte/icons';
	import { onDestroy, onMount } from 'svelte';
	import Trigger from '$lib/components/tasks/Trigger.svelte';
	import Steps from '$lib/components/tasks/Steps.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import { newSaveMonitor } from '$lib/save.js';

	interface Props {
		task: Task;
		project: Project;
		onChanged?: (task: Task) => void | Promise<void>;
	}

	let { task = $bindable(), onChanged, project }: Props = $props();

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
	}

	onDestroy(() => {
		saver.stop();
	});

	onMount(async () => {
		task = await ChatService.getTask(project.assistantID, project.id, task.id);
		saver.start();
	});
</script>

<div class="flex w-full justify-center overflow-y-auto">
	<!-- div in div is needed for the scrollbar to work so that space outside the max-width is still scrollable -->
	<div
		role="none"
		onkeydown={(e) => e.stopPropagation()}
		class="relative flex w-full max-w-[1200px] flex-col rounded-s-3xl p-5 scrollbar-none"
	>
		<input class="colors-background text-xl font-semibold outline-none" bind:value={task.name} />

		<input
			class="mt-0.5 bg-white text-sm outline-none dark:bg-black dark:text-gray-50"
			bind:value={task.description}
			placeholder="Enter description"
		/>

		<div>
			<Trigger bind:task />

			<Steps
				bind:task
				{project}
				save={async () => {
					await saver.save();
				}}
			/>
		</div>

		<div class="absolute right-0 top-0 m-2 flex">
			<button class="icon-button" onclick={() => (toDelete = true)}>
				<Trash class="h-5 w-5" />
			</button>
		</div>

		<div class="m-2 grow place-content-end self-end text-gray-300">id: {task.id}</div>
	</div>

	<Confirm
		show={toDelete}
		msg={`Are you sure you want to delete this task`}
		onsuccess={deleteTask}
		oncancel={() => (toDelete = false)}
	/>
</div>

<script lang="ts">
	import { Plus, Trash } from 'lucide-svelte/icons';
	import { tasks } from '$lib/stores';
	import { EditorService, type Task } from '$lib/services';
	import Confirm from '$lib/components/Confirm.svelte';
	import { CheckSquare } from 'lucide-svelte';
	import Menu from '$lib/components/navbar/Menu.svelte';

	async function deleteTask() {
		if (!taskToDelete?.id) {
			return;
		}
		await tasks.remove(taskToDelete.id);
		EditorService.remove(taskToDelete.id);
		menu?.open.set(false);
		taskToDelete = undefined;
	}

	async function newTask() {
		const task = await tasks.create();
		await EditorService.load(task.id);
		menu?.open.set(false);
	}

	let taskToDelete = $state<Task | undefined>();
	let menu = $state<ReturnType<typeof Menu>>();
</script>

<Menu bind:this={menu} title="Tasks" description="Helpful automations" onLoad={tasks.reload}>
	{#snippet icon()}
		<CheckSquare class="h-5 w-5" />
	{/snippet}
	{#snippet body()}
		{#if tasks.items.length === 0}
			<p class="p-6 text-center text-sm text-gray dark:text-gray-300">No tasks</p>
		{:else}
			<ul class="space-y-4 py-6 text-sm">
				{#each tasks.items.values() as task}
					<li class="group">
						<div class="flex">
							<button
								class="flex flex-1 items-center"
								onclick={async () => {
									await EditorService.load(task.id);
									menu?.open.set(false);
								}}
							>
								<CheckSquare class="h-5 w-5" />
								<span class="ms-3">{task.name}</span>
							</button>
							<button
								class="hidden group-hover:block"
								onclick={() => {
									taskToDelete = task;
								}}
							>
								<Trash class="h-5 w-5 text-gray-400" />
							</button>
						</div>
					</li>
				{/each}
			</ul>
		{/if}
		<div class="flex justify-end">
			<button
				onclick={newTask}
				class="-mb-3 -mr-3 mt-3 flex items-center justify-end gap-2 rounded-3xl p-3 px-4 hover:bg-gray-500 hover:text-white"
			>
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

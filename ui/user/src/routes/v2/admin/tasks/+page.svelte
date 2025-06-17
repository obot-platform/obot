<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { AdminService } from '$lib/services/index.js';
	import { Puzzle, Trash2 } from 'lucide-svelte';
	import { fade } from 'svelte/transition';

	let { data } = $props();
	const { threads, users, tasks: initialTasks, projects } = data;
	let loading = $state(false);

	let tasks = $state(initialTasks);
	let taskRuns = $derived(threads.filter((thread) => thread.taskID && !thread.deleted));
	let threadCounts = $derived(
		taskRuns.reduce<Record<string, number>>((acc, thread) => {
			if (!thread.taskID) return acc;

			if (!acc[thread.taskID]) {
				acc[thread.taskID] = 0;
			}
			acc[thread.taskID]++;
			return acc;
		}, {})
	);
	let taskOwners = $derived(
		new Map(
			taskRuns.map((thread) => [
				thread.taskID,
				users.find((user) => user.id === thread.userID)?.email ?? 'Unknown'
			])
		)
	);

	let taskTableData = $derived(
		tasks.map((task) => ({
			...task,
			project: projects.find((project) => project.id === task.projectID)?.name ?? 'Unknown',
			threadCount: threadCounts[task.id] ?? 0,
			createdBy: taskOwners.get(task.id) ?? 'Unknown'
		}))
	);
	let deletingTask = $state<(typeof taskTableData)[0]>();

	const duration = PAGE_TRANSITION_DURATION;
</script>

<Layout>
	<div class="my-4" in:fade={{ duration }} out:fade={{ duration }}>
		<div class="flex flex-col gap-8">
			<h1 class="text-2xl font-semibold">Tasks</h1>
			{#if taskTableData.length === 0}
				<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
					<Puzzle class="size-24 text-gray-200 dark:text-gray-900" />
					<h4 class="text-lg font-semibold text-gray-400 dark:text-gray-600">No created tasks</h4>
					<p class="text-sm font-light text-gray-400 dark:text-gray-600">
						Tasks are AI processes that are created by the user to perform a specific task within
						Chat.
					</p>
				</div>
			{:else}
				<Table
					data={taskTableData}
					fields={['id', 'project', 'threadCount', 'createdBy', 'created']}
					headers={[
						{ title: 'Runs', property: 'threadCount' },
						{ title: 'Created By', property: 'createdBy' }
					]}
				>
					{#snippet actions(d)}
						<button
							class="icon-button hover:text-red-500"
							onclick={(e) => {
								e.stopPropagation();
								deletingTask = d;
							}}
							use:tooltip={'Delete Task'}
						>
							<Trash2 class="size-4" />
						</button>
					{/snippet}
				</Table>
			{/if}
		</div>
	</div>
</Layout>

<Confirm
	msg={`Are you sure you want to delete this project?`}
	show={Boolean(deletingTask)}
	onsuccess={async () => {
		if (!deletingTask) return;
		loading = true;
		tasks = await AdminService.listTasks();
		loading = false;
		deletingTask = undefined;
	}}
	oncancel={() => (deletingTask = undefined)}
	{loading}
/>

<svelte:head>
	<title>Obot | Tasks</title>
</svelte:head>

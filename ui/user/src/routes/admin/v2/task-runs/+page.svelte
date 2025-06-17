<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { AdminService } from '$lib/services/index.js';
	import { Cpu, Puzzle, Trash2 } from 'lucide-svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	const { threads: initialThreads, tasks } = data;
	let loading = $state(false);

	let threads = $state(initialThreads);
	let taskRuns = $derived(threads.filter((thread) => thread.taskID && !thread.deleted));

	let taskMap = $derived(new Map(tasks.map((task) => [task.id, task])));

	let taskRunsTableData = $derived(
		taskRuns.map((taskRun) => ({
			...taskRun,
			task: taskMap.get(taskRun.taskID!)?.name ?? 'Unknown'
		}))
	);
	let deletingTaskRun = $state<(typeof taskRunsTableData)[0]>();

	const duration = PAGE_TRANSITION_DURATION;
</script>

<Layout>
	<div class="my-8" in:fly={{ x: 100, duration }} out:fly={{ x: -100, duration }}>
		<div
			class="flex flex-col gap-8"
			in:fly={{ x: 100, delay: duration, duration }}
			out:fly={{ x: -100, duration }}
		>
			<h1 class="text-2xl font-semibold">Task Runs</h1>
			{#if taskRunsTableData.length === 0}
				<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
					<Cpu class="size-24 text-gray-200 dark:text-gray-900" />
					<h4 class="text-lg font-semibold text-gray-400 dark:text-gray-600">
						No created task runs
					</h4>
					<p class="text-sm font-light text-gray-400 dark:text-gray-600">
						Task runs are the results of a task being run by a user.
					</p>
				</div>
			{:else}
				<Table data={taskRunsTableData} fields={['id', 'task', 'created']}>
					{#snippet actions(d)}
						<button
							class="icon-button hover:text-red-500"
							onclick={(e) => {
								e.stopPropagation();
								deletingTaskRun = d;
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
	show={Boolean(deletingTaskRun)}
	onsuccess={async () => {
		if (!deletingTaskRun) return;
		loading = true;
		// tasks = await AdminService.listTasks();
		loading = false;
		deletingTaskRun = undefined;
	}}
	oncancel={() => (deletingTaskRun = undefined)}
	{loading}
/>

<svelte:head>
	<title>Obot | Task Runs</title>
</svelte:head>

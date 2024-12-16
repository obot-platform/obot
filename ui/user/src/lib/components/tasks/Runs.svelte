<script lang="ts">
	import { Info, Play } from 'lucide-svelte';
	import { ChatService, type TaskRun } from '$lib/services';
	import { onDestroy } from 'svelte';
	import { currentAssistant } from '$lib/stores';
	import { Trash } from '$lib/icons';
	import { formatTime } from '$lib/time';
	import Modal from '$lib/components/Modal.svelte';

	interface Props {
		id: string;
		onSelect?: (runId: string) => void | Promise<void>;
	}

	let { id, onSelect }: Props = $props();
	let runs: TaskRun[] = $state([]);
	let timeout: number;
	let selected = $state('');
	let toDelete: string = $state('');

	onDestroy(() => {
		if (timeout) {
			clearTimeout(timeout);
		}
	});

	currentAssistant.subscribe((assistant) => {
		if (assistant.id) {
			listRuns();
		}
	});

	async function select(runID: string) {
		if (selected === runID) {
			runID = '';
		}
		await onSelect?.(runID);
		selected = runID;
	}

	async function listRuns() {
		try {
			if ($currentAssistant.id && id) {
				runs = (await ChatService.listTaskRuns($currentAssistant.id, id)).items;
			}
		} finally {
			if (timeout) {
				clearTimeout(timeout);
				timeout = 0;
			}
			timeout = setTimeout(listRuns, 5000);
		}
	}

	async function run() {
		if ($currentAssistant.id && id) {
			const newRun = await ChatService.runTask($currentAssistant.id, id);
			runs = (await ChatService.listTaskRuns($currentAssistant.id, id)).items;
			await select(newRun.id);
		}
	}

	async function deleteTask(runId: string) {
		if ($currentAssistant.id && id) {
			await ChatService.deleteTaskRun($currentAssistant.id, id, runId);
			runs = (await ChatService.listTaskRuns($currentAssistant.id, id)).items;
			if (selected === runId) {
				await select(runId);
			}
		}
	}

	function formatInput(task: TaskRun) {
		if (!task.input) {
			return '';
		}
		try {
			const input = JSON.parse(task.input);
			if (input.type === 'email') {
				return `${input.from}: ${input.subject}`;
			} else if (input.type === 'webhook') {
				return input.payload.slice(0, 50);
			} else if (typeof input === 'object') {
				return task.input.slice(0, 50);
			}
		} catch {
			return task.input.slice(0, 50);
		}
	}
</script>

{#snippet runButton()}
	<button
		class="flex items-center gap-2 rounded-3xl bg-blue px-5 py-2 text-white hover:bg-blue-400"
		onclick={run}
	>
		Run now
		<Play class="h-5 w-5" />
	</button>
{/snippet}

{#if runs.length > 0}
	<div class="mt-8 rounded-3xl bg-gray-50 p-5 dark:bg-gray-950">
		<h4 class="mb-3 text-xl font-semibold">Runs</h4>
		<table class="m-5 text-left">
			<thead class="font-semibold">
				<tr>
					<th class="pb-1 pl-2"> Start </th>
					<th> Input </th>
					<th class="pb-1 pl-6"> Duration </th>
					<th></th>
				</tr>
			</thead>
			<tbody>
				{#each runs as run}
					<tr class="group hover:cursor-pointer" onclick={() => select(run.id)}>
						<td
							class="rounded-s-md pl-2 group-hover:bg-gray-800"
							class:bg-blue={selected === run.id}
						>
							{formatTime(run.created)}
						</td>
						<td class="pl-6 group-hover:bg-gray-800" class:bg-blue={selected === run.id}>
							{formatInput(run)}
						</td>
						<td class="pl-6 group-hover:bg-gray-800" class:bg-blue={selected === run.id}>
							{#if run.startTime && run.endTime}
								{Math.round(
									(new Date(run.endTime).getTime() - new Date(run.startTime).getTime()) / 1000
								)}s
							{:else if run.startTime}
								Running
							{:else}
								Queued
							{/if}
						</td>
						<td
							class="rounded-e-md pl-6 group-hover:bg-gray-800 group-hover:text-gray"
							class:bg-blue={selected === run.id}
						>
							<div
								class="flex items-center gap-2 pl-2 text-gray"
								class:text-white={selected === run.id}
							>
								<Info class="h-4 w-4" />
								<button onclick={() => (toDelete = run.id)}>
									<Trash class="h-5 w-5" />
								</button>
							</div>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
		<div class="mt-8 flex justify-end">
			{@render runButton()}
		</div>
	</div>
{:else}
	<div class="mt-8 flex justify-end">
		{@render runButton()}
	</div>
{/if}

<Modal
	show={toDelete !== ''}
	msg={`Are you sure you want to delete this task run`}
	onsuccess={() => {
		deleteTask(toDelete);
		toDelete = '';
	}}
	oncancel={() => (toDelete = '')}
/>

<style lang="postcss">
	td,
	th {
		@apply p-1.5 px-6;
	}
</style>

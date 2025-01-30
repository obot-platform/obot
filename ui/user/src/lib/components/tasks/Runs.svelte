<script lang="ts">
	import { Info, OctagonX, Play, X } from 'lucide-svelte';
	import { ChatService, type Task, type TaskRun } from '$lib/services';
	import { onDestroy, onMount } from 'svelte';
	import { Trash } from 'lucide-svelte/icons';
	import { formatTime } from '$lib/time';
	import Confirm from '$lib/components/Confirm.svelte';
	import Input from '$lib/components/tasks/Input.svelte';
	import { assistants } from '$lib/stores/index';

	interface Props {
		id: string;
		onSelect?: (runId: string) => void | Promise<void>;
	}

	let { id, onSelect }: Props = $props();
	let runs: TaskRun[] = $state([]);
	let timeout: number;
	let selected = $state('');
	let toDelete: string = $state('');
	let inputDialog = $state<HTMLDialogElement>();
	let taskToRun = $state<Task>();
	let taskInput = $state('');

	onDestroy(() => {
		if (timeout) {
			clearTimeout(timeout);
		}
	});

	onMount(() => {
		if (assistants.current().id) {
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
			if (assistants.current().id && id) {
				runs = (await ChatService.listTaskRuns(id)).items;
			}
		} finally {
			if (timeout) {
				clearTimeout(timeout);
				timeout = 0;
			}
			timeout = setTimeout(listRuns, 5000);
		}
	}

	async function run(withInput?: string) {
		if (!withInput) {
			taskToRun = await ChatService.getTask(id);
			if (taskToRun.onDemand?.params && Object.keys(taskToRun.onDemand.params).length > 0) {
				inputDialog?.showModal();
				return;
			}
			if (taskToRun.webhook) {
				inputDialog?.showModal();
				return;
			}
		}

		inputDialog?.close();
		if (assistants.current().id && id) {
			const newRun = await ChatService.runTask(id, {
				input: withInput
			});
			runs = (await ChatService.listTaskRuns(id)).items;
			await select(newRun.id);
		}
	}

	async function abort(runId: string) {
		if (assistants.current().id && id) {
			await ChatService.abort({
				taskID: id,
				runID: runId
			});
			runs = (await ChatService.listTaskRuns(id)).items;
		}
	}

	async function deleteTask(runId: string) {
		if (assistants.current().id && id) {
			await ChatService.deleteTaskRun(id, runId);
			runs = (await ChatService.listTaskRuns(id)).items;
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
				return Object.keys(input)
					.map((key) => {
						return `${key}=${input[key]}`;
					})
					.join(', ')
					.slice(0, 50);
			}
		} catch {
			return task.input.slice(0, 50);
		}
	}
</script>

{#snippet runButton(opts?: { input?: string; text?: string })}
	<button
		class="flex items-center gap-2 rounded-3xl bg-blue px-5 py-2 text-white hover:bg-blue-400"
		onclick={() => {
			run(opts?.input);
		}}
	>
		{opts?.text ?? 'Run now'}
		<Play class="h-5 w-5" />
	</button>
{/snippet}

{#if runs.length > 0}
	<div class="mt-8 rounded-3xl bg-gray-50 p-5 dark:bg-gray-950">
		<div class="mb-3 flex items-center">
			<h4 class="flex-1 text-xl font-semibold">Runs</h4>
			{@render runButton()}
		</div>
		<table class="w-full text-left">
			<thead class="font-semibold">
				<tr>
					<th> Start</th>
					<th> Input</th>
					<th> Duration</th>
					<th></th>
				</tr>
			</thead>
			<tbody>
				{#each runs as run}
					<tr class="group hover:cursor-pointer" onclick={() => select(run.id)}>
						<td
							class="rounded-s-md group-hover:bg-gray-100 dark:group-hover:bg-gray-800"
							class:bg-blue={selected === run.id}
						>
							{formatTime(run.created)}
						</td>
						<td
							class="group-hover:bg-gray-100 dark:group-hover:bg-gray-800"
							class:bg-blue={selected === run.id}
						>
							{formatInput(run)}
						</td>
						<td
							class="group-hover:bg-gray-100 dark:group-hover:bg-gray-800"
							class:bg-blue={selected === run.id}
						>
							{#if run.error}
								{run.error.includes('aborted') ? 'Aborted' : run.error}
							{:else if run.startTime && run.endTime}
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
							class="rounded-e-md group-hover:bg-gray-100 group-hover:text-gray dark:group-hover:bg-gray-800"
							class:bg-blue={selected === run.id}
						>
							<div class="flex items-center gap-2 text-gray" class:text-white={selected === run.id}>
								{#if !run.error && run.startTime && !run.endTime}
									<button class="flex items-center gap-1" onclick={() => abort(run.id)}>
										<OctagonX class="h-4 w-4" />
										Stop
									</button>
								{/if}
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
	</div>
{:else}
	<div class="mt-8 flex justify-end">
		{@render runButton()}
	</div>
{/if}

<dialog
	bind:this={inputDialog}
	class="relative rounded-3xl border-white bg-white p-5 text-black dark:bg-black dark:text-gray-50 md:min-w-[500px]"
>
	<h4 class="text-xl font-semibold">Arguments</h4>
	<Input editMode task={taskToRun} bind:input={taskInput}></Input>
	<div class="mt-5 flex w-full justify-end">
		{@render runButton({
			input: taskInput,
			text: 'Run'
		})}
	</div>
	<button
		class="absolute right-0 top-0 p-5 text-sm text-gray dark:text-gray-400"
		onclick={() => {
			inputDialog?.close();
		}}
	>
		<X class="h-5 w-5" />
	</button>
</dialog>

<Confirm
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
		@apply p-1.5;
	}

	dialog::backdrop {
		@apply bg-black bg-opacity-60;
	}
</style>

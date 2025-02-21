<script lang="ts">
	import Self from './Step.svelte';
	import {
		ChatService,
		type Messages,
		type Project,
		type Task,
		type TaskStep
	} from '$lib/services';
	import Message from '$lib/components/messages/Message.svelte';
	import { Plus, Trash } from 'lucide-svelte/icons';
	import { LoaderCircle, OctagonX, Play, RefreshCcw, Save, Undo } from 'lucide-svelte';
	import { tick } from 'svelte';
	import { autoHeight } from '$lib/actions/textarea.js';
	import Confirm from '$lib/components/Confirm.svelte';

	interface Props {
		parentStale?: boolean;
		onChange?: (steps: TaskStep[]) => void | Promise<void>;
		run?: (step: TaskStep, steps?: TaskStep[]) => Promise<void>;
		task: Task;
		index: number;
		pending?: boolean;
		editMode?: boolean;
		stepMessages?: Map<string, Messages>;
		project: Project;
	}

	let {
		parentStale,
		onChange,
		run,
		task,
		index,
		editMode = false,
		pending,
		stepMessages,
		project
	}: Props = $props();

	let step = $derived(task.steps[index]);
	let messages = $derived(stepMessages?.get(step.id)?.messages ?? []);
	let lastSeenValue: string | undefined = $state();
	let currentValue = $state(task.steps[index].step);
	let dirty = $derived(task.steps[index].step !== currentValue);
	let stale: boolean = $derived(dirty || parentStale || !parentMatches());
	let running = $derived(stepMessages?.get(step.id)?.inProgress ?? false);
	let toDelete: boolean | undefined = $state();
	let nextStep: Self | undefined = $state();

	$effect(() => {
		if (editMode) {
			if (lastSeenValue !== step.step) {
				currentValue = step.step;
				lastSeenValue = step.step;
			}
		} else {
			if (currentValue !== step.step) {
				currentValue = step.step;
			}
			if (lastSeenValue !== '') {
				lastSeenValue = '';
			}
		}
	});

	function parentMatches() {
		if (running) {
			return true;
		}
		if (index === 0) {
			return true;
		}
		const lastRun = stepMessages
			?.get(task.steps[index - 1].id)
			?.messages.findLast((msg) => msg.runID);
		const currentRun = stepMessages
			?.get(task.steps[index].id)
			?.messages.find((msg) => msg.parentRunID);
		return lastRun?.runID === currentRun?.parentRunID;
	}

	async function deleteStep() {
		toDelete = undefined;
		const newSteps = [...task.steps];
		newSteps.splice(index, 1);
		await onChange?.(newSteps);
	}

	async function revert() {
		if (dirty) {
			currentValue = step.step;
		}
	}

	function synchronized(newSteps?: TaskStep[]): TaskStep[] | undefined {
		if (!newSteps && !dirty) {
			return;
		}

		const retSteps = newSteps ?? [...task.steps];
		if (dirty) {
			retSteps[index] = {
				...step,
				step: currentValue
			};
		}

		return retSteps;
	}

	export async function saveAll() {
		await save();
		if (nextStep) {
			await nextStep.saveAll();
		}
	}

	async function save(steps?: TaskStep[]) {
		const newSteps = synchronized(steps);
		if (newSteps) {
			await onChange?.(newSteps);
		}
	}

	async function addStep() {
		const newStep = createStep();
		const newSteps = [...task.steps];
		newSteps.splice(index + 1, 0, newStep);
		await save(newSteps);
		await tick();
		document.getElementById('step' + newStep.id)?.focus();
	}

	async function onkeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.ctrlKey && !e.shiftKey) {
			e.preventDefault();
			await doRun();
		} else if (e.key === 'Enter' && e.ctrlKey && !e.shiftKey) {
			e.preventDefault();
			await addStep();
		}
	}

	function createStep(): TaskStep {
		return { id: Math.random().toString(36).substring(7), step: '' };
	}

	async function doRun() {
		if ((running || pending) && editMode) {
			await ChatService.abort(project.assistantID, project.id, {
				taskID: task.id,
				runID: 'editor'
			});
			return;
		}
		if (running || pending || !currentValue || currentValue?.trim() === '') {
			return;
		}
		await run?.(step, synchronized());
	}
</script>

<li class="ms-6 marker:font-semibold">
	<div class="flex items-center justify-between">
		{#if editMode}
			<textarea
				{onkeydown}
				rows="1"
				placeholder="Instructions..."
				use:autoHeight
				id={'step' + step.id}
				bind:value={currentValue}
				class="flex-1 resize-none border-none bg-gray-50 outline-none dark:bg-gray-950"
			></textarea>
			<div class="flex gap-2 p-2">
				<button class="rounded-lg p-2 hover:bg-gray-50 dark:hover:bg-gray-950" onclick={doRun}>
					{#if running}
						<OctagonX class="h-4 w-4" />
					{:else if pending}
						<LoaderCircle class="h-4 w-4 animate-spin" />
					{:else if messages.length > 0}
						<RefreshCcw class="h-4 w-4" />
					{:else}
						<Play class="h-4 w-4" />
					{/if}
				</button>
				{#if dirty}
					<button
						class="-ml-3 rounded-lg p-2 hover:bg-gray-50 dark:hover:bg-gray-950"
						onclick={revert}
					>
						<Undo class="h-4 w-4" />
					</button>
					<button
						class="-ml-3 rounded-lg p-2 hover:bg-gray-50 dark:hover:bg-gray-950"
						onclick={async () => {
							await save();
						}}
					>
						<Save class="h-4 w-4" />
					</button>
				{/if}
				<button
					class="-ml-3 rounded-lg p-2 hover:bg-gray-50 dark:hover:bg-gray-950"
					onclick={() => {
						if (currentValue?.trim() === '') {
							deleteStep();
						} else {
							toDelete = true;
						}
					}}
				>
					<Trash class="h-4 w-4" />
				</button>
				{#if currentValue?.trim() !== ''}
					<button
						class="-ml-3 rounded-lg p-2 hover:bg-gray-50 dark:hover:bg-gray-950"
						onclick={addStep}
					>
						<Plus class="h-4 w-4" />
					</button>
				{/if}
			</div>
		{:else}
			<span>{currentValue}</span>
		{/if}
	</div>
	{#if messages.length > 0}
		<div
			class="relative my-3 -ml-6 rounded-3xl bg-white p-5 transition-transform dark:bg-black"
			class:border-2={running}
			class:border-blue={running}
		>
			{#each messages as msg}
				{#if !msg.sent}
					<Message {msg} {project} />
				{/if}
			{/each}
			{#if stale}
				<div
					class="absolute inset-0 h-full w-full rounded-3xl bg-white opacity-80 dark:bg-black"
				></div>
			{/if}
		</div>
	{/if}
</li>

{#if task.steps.length > index + 1}
	{#key task.steps[index + 1].id}
		<Self
			bind:this={nextStep}
			{onChange}
			{run}
			{pending}
			{editMode}
			{task}
			index={index + 1}
			{stepMessages}
			parentStale={stale}
			{project}
		/>
	{/key}
{/if}

<Confirm
	show={toDelete !== undefined}
	msg={`Are you sure you want to delete this step`}
	onsuccess={deleteStep}
	oncancel={() => (toDelete = undefined)}
/>

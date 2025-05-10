<script lang="ts">
	import {
		ChatService,
		type Messages,
		type Project,
		type Task,
		type TaskStep
	} from '$lib/services';
	import Message from '$lib/components/messages/Message.svelte';
	import { Plus, Trash2, Repeat } from 'lucide-svelte/icons';
	import { LoaderCircle, OctagonX, Play, RefreshCcw } from 'lucide-svelte';
	import { tick, untrack } from 'svelte';
	import { autoHeight } from '$lib/actions/textarea.js';
	import Confirm from '$lib/components/Confirm.svelte';
	import { fade, slide } from 'svelte/transition';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import LoopStep from './LoopStep.svelte';
	import { transitionParentHeight } from '$lib/actions/size.svelte';
	import { linear } from 'svelte/easing';

	interface Props {
		parentStale?: boolean;
		run?: (step: TaskStep) => Promise<void>;
		task: Task;
		index: number;
		step: TaskStep;
		runID?: string;
		pending?: boolean;
		stepMessages?: Map<string, Messages>;
		project: Project;
		showOutput?: boolean;
		readOnly?: boolean;
	}

	let {
		parentStale,
		run,
		task = $bindable(),
		index,
		step = $bindable(),
		runID,
		pending,
		stepMessages,
		project,
		showOutput: parentShowOutput,
		readOnly
	}: Props = $props();

	// let isRunning = $derived(stepMessages?.get(step.id)?.inProgress ?? false);
	let isRunnedBefore = $derived(!!stepMessages?.get(step.id)?.lastRunID);
	let stale: boolean = $derived(parentStale || !parentMatches());
	let toDelete = $state<boolean>();
	let showOutput = $state(true);

	let isRunning = $state(stepMessages?.get(step.id)?.inProgress ?? false);

	let timeoutId: number | undefined = undefined;
	// save how many step.inProgress === false we got
	let inProgressFalseCount = $state(0);
	$effect(() => {
		const s = stepMessages?.get(step.id);

		untrack(() => {
			clearTimeout(timeoutId);

			// check if inProgress is false
			if (!s?.inProgress) {
				// increment the counter
				inProgressFalseCount++;

				// check if we got 2 false responses
				if (inProgressFalseCount > 2) {
					// set as not running
					isRunning = false;

					inProgressFalseCount = 0;
				}

				// in case we got the last message and 1 false inProgress; set a timeout function to update isRunning after some time
				timeoutId = setTimeout(() => {
					isRunning = false;
					inProgressFalseCount = 0;
				}, 1000);
			} else {
				// set task as running
				isRunning = true;

				inProgressFalseCount = 0;
			}
		});
	});

	// Check whether the current step has looping steps (sub steps)
	let isLoopStep = $derived((step?.loop?.length ?? 0) > 0);

	let messages = $derived(stepMessages?.get(step.id)?.messages ?? []);

	let loopDataMessages = $derived(stepMessages?.get(step.id + '{loopdata}')?.messages ?? []);

	const taskRunStepLoopProgress = $derived(
		getTaskRunProgress(step.id, stepMessages?.keys().toArray() ?? [])
	);

	type Iteration = Messages[];

	// Convert the steps messages map to an array of messages where each index represent the number of iteration
	let iterations: Iteration[] = $derived.by(() => {
		// Convert the keys into an array
		const keys = stepMessages?.keys().toArray() ?? [];

		// Define a regex pattern to extract iterations data
		const pattern = new RegExp(`^${step.id}{element=(\\d+)}`);

		// Initialize the iterations array
		const iterations: Iteration[] = [];

		keys
			// Filter out not matched items
			.filter((key) => pattern.test(key))

			.forEach((key) => {
				// Get the iteration number as a string
				const iterationAsString = key.match(pattern)?.at(1);

				if (iterationAsString === undefined) {
					return;
				}

				// Convert the iteration number to an integer
				const iteration = parseInt(iterationAsString);

				// Push the step messages to the same iteration array
				const steps = iterations.at(iteration) ?? [];
				const messages = stepMessages?.get(key);

				steps.push(messages!);

				iterations[iteration] = steps;
			});

		return iterations;
	});

	$effect(() => {
		if (parentShowOutput !== undefined) {
			showOutput = parentShowOutput;
		}
	});

	function parentMatches() {
		if (isRunning) {
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

	async function toggleLoop() {
		if (isLoopStep) {
			step.loop = undefined;
		} else {
			step.loop = [''];
		}
	}

	async function deleteStep() {
		task.steps = task.steps.filter((s) => s.id !== step.id);
	}

	async function addStep() {
		const newStep = createStep();
		task.steps.splice(index + 1, 0, newStep);
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
		if (isRunning || pending) {
			if (runID) {
				await ChatService.abort(project.assistantID, project.id, {
					taskID: task.id,
					runID: runID
				});
			}
			return;
		}
		if (isRunning || pending || !step.step || step.step?.trim() === '') {
			return;
		}

		await run?.($state.snapshot(step));
	}

	function getTaskRunProgress(stepId: string, messageIds: string[]) {
		// Define a regex pattern to extract iterations data
		const pattern = new RegExp(`^${stepId}{element=(\\d+)}{step=(\\d+)}`);

		let acc = { iteration: -1, loopStep: -1 };

		for (const key of messageIds) {
			if (!pattern.test(key)) {
				continue;
			}

			const match = key.match(pattern);
			const iteration = parseInt(match?.at(1) ?? '0');
			const loopStep = parseInt(match?.at(2) ?? '0');

			acc = {
				iteration: Math.max(acc.iteration, iteration),
				loopStep: loopStep
			};
		}

		return acc;
	}
</script>

<li class="ms-4">
	<div class="flex items-start justify-between gap-6">
		<div class="flex grow flex-col gap-2">
			<div class="flex items-center gap-2">
				<textarea
					{onkeydown}
					rows="1"
					placeholder={isLoopStep ? 'Description of the data to loop over...' : 'Instructions...'}
					use:autoHeight
					id={'step' + step.id}
					bind:value={step.step}
					class="ghost-input border-surface2 ml-1 grow resize-none"
					disabled={readOnly}
				></textarea>
			</div>

			{#if isLoopStep}
				{#if loopDataMessages.length > 0 && showOutput}
					<!-- Show step message -->
					<div
						class="transition-height relative my-3 -ml-4 box-content flex min-h-6 flex-col gap-4 overflow-hidden rounded-lg bg-white p-5 dark:bg-black"
						class:outline-2={isRunning}
						class:outline-blue={isRunning}
						transition:slide={{ duration: !readOnly ? 200 : 0, easing: linear }}
					>
						<div
							class="message-container flex w-full flex-col gap-4"
							use:transitionParentHeight={() => (isRunning && showOutput) || loopDataMessages}
						>
							{#each loopDataMessages as msg}
								{#if !msg.sent}
									<Message {msg} {project} disableMessageToEditor />
								{/if}
							{/each}
						</div>
						{#if stale}
							<div
								class="absolute inset-0 h-full w-full rounded-3xl bg-white opacity-80 dark:bg-black"
							></div>
						{/if}
					</div>
				{/if}

				<div class="iterations-body flex flex-col gap-2">
					{#if iterations.length && (isRunning || isRunnedBefore)}
						{#each iterations as iteration, i}
							<!-- Get the current iteration steps messages array -->
							{@const messages = iteration ?? []}

							<div class="iteration flex flex-col">
								<div class="flex py-2">
									<div class="text-lg font-semibold">
										<span>Iteration</span>
										<span>{i + 1}</span>
									</div>
								</div>

								{#each step.loop! as _, j (j)}
									<!-- Get the current step messages array -->
									{@const stepMessages = messages[j] ?? []}

									<LoopStep
										bind:value={step.loop![j]}
										{project}
										messages={stepMessages}
										isReadOnly={readOnly}
										isLoopStepRunning={isRunning &&
											taskRunStepLoopProgress &&
											taskRunStepLoopProgress.iteration === i &&
											taskRunStepLoopProgress.loopStep === j}
										isStepRunning={isRunning}
										isStepRunned={isRunnedBefore}
										shouldShowOutput={showOutput}
										{stale}
										onKeydown={onkeydown}
										onDelete={() => step.loop!.splice(j, 1)}
										index={j}
									/>
								{/each}
							</div>
						{/each}
					{:else}
						<div class="flex flex-col gap-2">
							{#if isRunning || isRunnedBefore}
								<div class="flex h-11 rounded-lg py-2">
									<div class="text-lg font-semibold opacity-30">Waiting for iteration data...</div>
								</div>
							{/if}

							{#each step.loop! as _, i}
								<!-- Get the current iteration steps messages array -->
								{@const messages = iterations[i] ?? []}

								<!-- Get the current step messages array -->
								{@const stepMessages = messages[i] ?? []}

								<LoopStep
									bind:value={step.loop![i]}
									{project}
									messages={stepMessages}
									isReadOnly={readOnly}
									isLoopStepRunning={false}
									isStepRunning={false}
									isStepRunned={false}
									shouldShowOutput={showOutput}
									{stale}
									onKeydown={onkeydown}
									onDelete={() => step.loop!.splice(i, 1)}
									onAdd={() => step.loop!.splice(i + 1, 0, '')}
								/>
							{/each}
						</div>
					{/if}
				</div>
			{/if}
		</div>

		{#if !readOnly}
			<div class="flex items-start">
				<button
					class="icon-button"
					class:text-blue={isLoopStep}
					data-testid="step-loop-btn"
					onclick={toggleLoop}
					use:tooltip={isLoopStep ? 'Convert to regular step' : 'Convert to loop step'}
				>
					<Repeat class="size-4" />
				</button>

				<button
					class="icon-button"
					data-testid="step-run-btn"
					onclick={doRun}
					use:tooltip={isRunning
						? 'Abort'
						: pending
							? 'Running...'
							: messages.length > 0
								? 'Re-run Step'
								: 'Run Step'}
				>
					{#if isRunning}
						<OctagonX class="size-4" />
					{:else if pending}
						<LoaderCircle class="size-4 animate-spin" />
					{:else if messages.length > 0}
						<RefreshCcw class="size-4" />
					{:else}
						<Play class="size-4" />
					{/if}
				</button>
				<button
					class="icon-button"
					data-testid="step-delete-btn"
					onclick={() => {
						if (step.step?.trim()) {
							toDelete = true;
						} else {
							deleteStep();
						}
					}}
					use:tooltip={'Delete Step'}
				>
					<Trash2 class="size-4" />
				</button>
				<div class="flex grow">
					<div class="size-10">
						{#if (step.step?.trim() || '').length > 0}
							<button
								class="icon-button"
								data-testid="step-add-btn"
								onclick={addStep}
								use:tooltip={'Add Step'}
								transition:fade={{ duration: 200 }}
							>
								<Plus class="size-4" />
							</button>
						{/if}
					</div>
				</div>
			</div>
		{/if}
	</div>
</li>

<!-- This code section is responsible for showing messages in a !loop task -->
{#if !isLoopStep && messages.length > 0 && showOutput}
	<div
		class="transition-height relative my-3 box-content flex min-h-6 flex-col gap-4 overflow-hidden rounded-lg bg-white p-5 dark:bg-black"
		class:outline-2={isRunning}
		class:outline-blue={isRunning}
		transition:slide={{ duration: !readOnly ? 200 : 0, easing: linear }}
	>
		<div
			class="messages-container flex w-full flex-col gap-4"
			use:transitionParentHeight={() => isRunning || messages}
		>
			{#each messages as msg}
				{#if !msg.sent}
					<Message {msg} {project} disableMessageToEditor />
				{/if}
			{/each}
		</div>
		{#if stale}
			<div
				class="absolute inset-0 h-full w-full rounded-3xl bg-white opacity-80 dark:bg-black"
			></div>
		{/if}
	</div>
{/if}

<!-- This code section show dialog to confirm task delete -->
<!-- REFACTOR: Move out to the Steps.svelte component; having one dialog shared with many steps is better than each steps has its own dialog-->
<Confirm
	show={toDelete !== undefined}
	msg={`Are you sure you want to delete this step`}
	onsuccess={deleteStep}
	oncancel={() => (toDelete = undefined)}
/>

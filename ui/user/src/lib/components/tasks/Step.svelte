<script lang="ts">
	import {
		ChatService,
		EditorService,
		type Messages,
		type Project,
		type Task,
		type TaskStep
	} from '$lib/services';
	import Message from '$lib/components/messages/Message.svelte';
	import { Plus, Trash2, Repeat } from 'lucide-svelte/icons';
	import { LoaderCircle, OctagonX, Play, RefreshCcw } from 'lucide-svelte';
	import { untrack } from 'svelte';
	import { autoHeight } from '$lib/actions/textarea.js';
	import Confirm from '$lib/components/Confirm.svelte';
	import { fade, slide } from 'svelte/transition';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import LoopStep from './LoopStep.svelte';
	import { transitionParentHeight } from '$lib/actions/size.svelte';
	import { linear } from 'svelte/easing';
	import { DraggableHandle, DraggableItem } from '../primitives/draggable';
	import { twMerge } from 'tailwind-merge';
	import Loading from '$lib/icons/Loading.svelte';
	import { getLayout } from '$lib/context/chatLayout.svelte';

	interface Props {
		run?: (step: TaskStep) => Promise<void>;
		task: Task;
		index: number;
		step: TaskStep;
		loopSteps?: string[];
		runID?: string;
		pending?: boolean;
		stepMessages?: Record<string, Messages>;
		project: Project;
		showOutput?: boolean;
		readOnly?: boolean;
		lastStepId?: string;
		isTaskRunning?: boolean;
		onChange?: (step: TaskStep) => void;
		onAdd?: () => void;
		onDelete?: () => void;
	}

	let {
		run,
		task = $bindable(),
		step,
		index,
		runID,
		pending,
		stepMessages,
		project,
		showOutput: shouldShowOutputGlobal,
		readOnly,
		lastStepId,
		isTaskRunning = false,
		onChange,
		onAdd,
		onDelete
	}: Props = $props();

	// let isRunning = $derived(stepMessages?.get(step.id)?.inProgress ?? false);
	let isRunnedBefore = $derived(!!stepMessages?.[step.id]?.lastRunID);
	let toDelete = $state<boolean>();
	let isConvertToRegularDialogShown = $state(false);

	let loopStepToDeleteIndex: number | undefined = $state(undefined);
	let isLoopStepDeleteDialogShown = $derived(typeof loopStepToDeleteIndex === 'number');

	let shouldShowOutputLocal = $state(!!shouldShowOutputGlobal);

	let shouldShowOutput = $derived(shouldShowOutputLocal && shouldShowOutputGlobal);

	let isRunning = $state(lastStepId?.includes(step.id) ?? false);

	let timeoutId: number | undefined = undefined;
	// save how many step.inProgress === false we got
	let inProgressFalseCount = $state(0);
	$effect(() => {
		const s = stepMessages?.[step.id];

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

	let simpleStepMessages = $derived(stepMessages?.[step.id]?.messages ?? []);
	let loopStepMessage = $derived(stepMessages?.[step.id + '{loopdata}']?.messages ?? []);

	const messages = $derived(!isLoopStep ? simpleStepMessages : loopStepMessage);

	let runningProgress = $state(getCurrentRunData(step.id, lastStepId));

	const layout = getLayout();

	$effect(() => {
		if (lastStepId && isRunning) {
			untrack(() => {
				const newVal = getCurrentRunData(step.id, lastStepId);

				if (newVal && newVal.iteration >= (runningProgress?.iteration ?? 0)) {
					runningProgress = newVal;
				}
			});
		} else if (!lastStepId) {
			untrack(() => (runningProgress = undefined));
		}
	});

	type Iteration = Messages[];

	// Convert the steps messages map to an array of messages where each index represent the number of iteration
	let iterations: Iteration[] = $derived.by(() => {
		// Convert the keys into an array
		const keys = Object.keys(stepMessages ?? {}) ?? [];

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
				const messages = stepMessages?.[key];

				steps.push(messages!);

				iterations[iteration] = steps;
			});

		return iterations.filter(Boolean);
	});

	async function toggleLoop() {
		if (isLoopStep) {
			isConvertToRegularDialogShown = true;
			return;
		}

		makeLooptep();
	}

	function makeRegularStep() {
		const s = $state.snapshot(step);
		s.loop = undefined;
		onChange?.(s);
		// Close the dialog
		isConvertToRegularDialogShown = false;
	}

	function makeLooptep() {
		const s = $state.snapshot(step);
		s.loop = [''];
		onChange?.(s);
	}

	async function onkeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.ctrlKey && !e.shiftKey) {
			e.preventDefault();
			await doRun();
		} else if (e.key === 'Enter' && e.ctrlKey && !e.shiftKey) {
			e.preventDefault();
			onAdd?.();
		}
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

		// Activate visibility for the current step
		shouldShowOutput = true;

		await run?.($state.snapshot(step));
	}

	function getCurrentRunData(stepId: string, key?: string) {
		if (key) {
			// Define a regex pattern to extract iterations data
			const pattern = new RegExp(`^${stepId}{element=(\\d+)}{step=(\\d+)}`);

			const match = key.match(pattern);

			if (match) {
				const iteration = parseInt(match?.at(1) ?? '0');
				const loopStep = parseInt(match?.at(2) ?? '0');

				return {
					iteration: iteration,
					loopStep: loopStep
				};
			}
		}

		return undefined;
	}

	function deleteLoopStep(index?: number) {
		if (index === undefined) return;

		const s = $state.snapshot(step);
		s.loop?.splice(index, 1);

		onChange?.(s);
	}
</script>

<DraggableItem
	as="li"
	class={twMerge(
		'w-full gap-2 rounded-md',
		toDelete && 'bg-surface1 dark:bg-surface2',
		isTaskRunning &&
			((index > 0 && !isRunning) || (index === 0 && messages.length && !isRunning)) &&
			'opacity-50'
	)}
	{index}
	id={step.id}
	data={step}
>
	<div class="flex w-full items-start justify-between gap-6">
		<div class="flex flex-1 grow flex-col gap-0">
			<div class="flex items-center gap-2">
				<textarea
					{onkeydown}
					rows="1"
					placeholder={isLoopStep ? 'Description of the data to loop over...' : 'Instructions...'}
					use:autoHeight
					id={'step' + step.id}
					value={step.step}
					class="ghost-input border-surface2 ml-1 grow resize-none"
					readonly={readOnly}
					oninput={(ev) => {
						const value = (ev.target as HTMLInputElement).value;

						// Get a snapshot of procied object
						const s = $state.snapshot(step);
						// Apply new changes
						s.step = value;
						// Send it upp to be saved
						onChange?.(s);
					}}
				></textarea>

				{#if !readOnly}
					<div class="flex items-start">
						<button
							class="icon-button"
							class:text-blue={isLoopStep}
							data-testid="step-loop-btn"
							onclick={toggleLoop}
							use:tooltip={isLoopStep
								? 'Convert to regular step'
								: 'Iterate through the results of this step'}
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
									: simpleStepMessages.length > 0
										? 'Re-run Step'
										: 'Run Step'}
						>
							{#if isRunning}
								<OctagonX class="size-4" />
							{:else if pending}
								<LoaderCircle class="size-4 animate-spin" />
							{:else if simpleStepMessages.length > 0}
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
									// deleteStep();
									onDelete?.();
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
										onclick={onAdd}
										use:tooltip={'Add Step'}
										transition:fade={{ duration: 200 }}
									>
										<Plus class="size-4" />
									</button>
								{/if}
							</div>
						</div>

						{#if !readOnly}
							<DraggableHandle class="aspect-square opacity-50" />
						{/if}
					</div>
				{/if}
			</div>

			{#if isLoopStep}
				<div class="loop-steps flex flex-col gap-2 pr-[200px]">
					{#each step.loop! ?? [] as _, i (i)}
						<!-- Get the current iteration steps messages array -->
						{@const messages = iterations[i] ?? []}

						<!-- Get the current step messages array -->
						{@const stepMessages = messages[i] ?? []}

						<LoopStep
							bind:value={
								() => step.loop![i],
								(v) => {
									const s = $state.snapshot(step);

									if (s.loop) {
										s.loop[i] = v;
									}

									onChange?.(s);
								}
							}
							class={twMerge(
								'rounded-md',
								loopStepToDeleteIndex === i && 'bg-surface1 dark:bg-surface2'
							)}
							{project}
							messages={stepMessages}
							isReadOnly={readOnly}
							isLoopStepRunning={false}
							isStepRunning={false}
							isStepRunned={false}
							{shouldShowOutput}
							onKeydown={onkeydown}
							onDelete={() => (loopStepToDeleteIndex = i)}
							onAdd={() => {
								const s = $state.snapshot(step);
								s.loop?.splice(i + 1, 0, '');

								onChange?.(s);
							}}
							{runID}
							taskID={task.id}
						/>
					{/each}
				</div>
			{/if}

			<!-- This code section is responsible for showing messages in a !loop task -->
			<!-- Show messages container when: -->
			<!-- Show output is active -->
			<!-- Task is running and this is the first step to display an indicator so user knows something is hapenning -->
			<!-- This step is running -->
			<!-- Messages array is not empty -->
			{#if shouldShowOutput && ((isTaskRunning && index == 0) || isRunning || messages.length)}
				{@const shouldShowOutline = isRunning || (isTaskRunning && !messages.length && index === 0)}

				<div
					class="transition-height relative my-3 -ml-4 box-content flex min-h-6 flex-col gap-4 overflow-hidden rounded-lg bg-white p-5 dark:bg-black"
					class:outline-2={shouldShowOutline}
					class:outline-blue={shouldShowOutline}
					transition:slide={{
						duration: 200,
						easing: linear
					}}
				>
					<div
						class="messages-container flex w-full flex-col gap-4"
						use:transitionParentHeight={() => isRunning || messages}
					>
						{#if messages.length || isRunning}
							{#each messages as msg, i (i)}
								{#if !msg.sent}
									<Message
										{msg}
										{project}
										disableMessageToEditor
										onLoadFile={(filename) => {
											EditorService.load(layout.items, project, filename, {
												taskID: task.id,
												runID: runID
											});
											layout.fileEditorOpen = true;
										}}
									/>
								{/if}
							{/each}
							<!-- Show a loading placeholder in the first step when task is running and messages array is empty -->
						{:else if index == 0 && !messages.length && isTaskRunning}
							<!-- Running placeholder -->
							<!-- Show only on the first step when we don have any messages yet and only when task is running -->
							<div
								class="flex items-center gap-2 text-sm font-semibold"
								transition:fade={{ duration: 100 }}
							>
								<div>loading...</div>
								<Loading class="size-4" />
							</div>
						{/if}
					</div>
				</div>
			{/if}

			{#if shouldShowOutput}
				<div
					class="iterations-body flex flex-col gap-4"
					transition:slide={{ duration: 300, easing: linear }}
				>
					{#if iterations.length && (isRunning || isRunnedBefore)}
						{#each iterations as iteration, i (i)}
							<!-- Get the current iteration steps messages array -->
							{@const messages = iteration ?? []}

							<div
								class="iteration border-surface2 -ml-4 flex flex-col rounded-lg border pt-4"
								in:fade|global={{ duration: 200 }}
								out:fade={{ duration: 0 }}
							>
								<div class="mb-2 flex px-4">
									<div class="text-lg font-semibold">
										<span>Iteration</span>
										<span>{i + 1}</span>
									</div>
								</div>

								<div class="flex flex-col">
									{#each step.loop! ?? [] as s, j (s)}
										<!-- Get the current step messages array -->
										{@const stepMessages = messages[j] ?? []}

										<LoopStep
											class="border-surface2 border-b pr-4 last:border-none"
											value={step.loop![j]}
											{project}
											messages={stepMessages}
											isReadOnly={readOnly}
											isLoopStepRunning={isRunning &&
												runningProgress?.iteration === i &&
												runningProgress?.loopStep === j}
											isStepRunning={isRunning}
											isStepRunned={isRunnedBefore}
											{isTaskRunning}
											{shouldShowOutput}
											onKeydown={onkeydown}
											onDelete={() => {
												step.loop = step.loop!.splice(j, 1);
											}}
											{runID}
											taskID={task.id}
										/>
									{/each}
								</div>
							</div>
						{/each}
					{/if}
				</div>
			{/if}
		</div>
	</div>
</DraggableItem>

<!-- This code section show dialog to confirm task delete -->
<!-- REFACTOR: Move out to the Steps.svelte component; having one dialog shared with many steps is better than each steps has its own dialog-->
<Confirm
	show={toDelete !== undefined}
	msg="Are you sure you want to delete this step?"
	onsuccess={() => onDelete?.()}
	oncancel={() => (toDelete = undefined)}
/>

<Confirm
	show={isConvertToRegularDialogShown}
	msg="Making this a regular step will delete all loop step, are you sure you want to continue?"
	onsuccess={makeRegularStep}
	oncancel={() => (isConvertToRegularDialogShown = false)}
/>

<Confirm
	show={isLoopStepDeleteDialogShown}
	msg="Are you sure you want to delete this loop step?"
	onsuccess={() => {
		deleteLoopStep(loopStepToDeleteIndex);
		loopStepToDeleteIndex = undefined;
	}}
	oncancel={() => (loopStepToDeleteIndex = undefined)}
/>

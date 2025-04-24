<script lang="ts">
	import {
		ChatService,
		type Message as MessageType,
		type Messages,
		type Project,
		type Task,
		type TaskStep
	} from '$lib/services';
	import Message from '$lib/components/messages/Message.svelte';
	import {
		Eye,
		EyeClosed,
		Plus,
		Trash2,
		Repeat,
		ArrowLeft,
		ArrowRight,
		GripVertical
	} from 'lucide-svelte/icons';
	import { LoaderCircle, OctagonX, Play, RefreshCcw } from 'lucide-svelte';
	import { tick } from 'svelte';
	import { autoHeight } from '$lib/actions/textarea.js';
	import Confirm from '$lib/components/Confirm.svelte';
	import { fade, slide } from 'svelte/transition';
	import { tooltip } from '$lib/actions/tooltip.svelte';

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

	let running = $derived(stepMessages?.get(step.id)?.inProgress ?? false);
	let stale: boolean = $derived(parentStale || !parentMatches());
	let toDelete = $state<boolean>();
	let showOutput = $state(true);
	let isLoopStep = $derived<boolean>(step.loop !== undefined);
	let messages = $derived(stepMessages?.get(step.id)?.messages ?? []);
	let loopDataMessages = $derived(stepMessages?.get(step.id + '{loopdata}')?.messages ?? []);

	type GroupedMessages = {
		step: number;
		messages: MessageType[];
	};
	function groupByStepAsArray(
		messagesMap: Map<string, { messages: MessageType[] }>
	): GroupedMessages[] {
		const grouped: Record<number, MessageType[]> = {};

		for (const [key, value] of messagesMap.entries()) {
			const match = key.match(/\{step=(\d+)\}/);
			if (match) {
				const step = Number(match[1]);
				if (!grouped[step]) {
					grouped[step] = [];
				}
				grouped[step].push(...value.messages);
			}
		}

		// Convert to array of { step, messages }
		return Object.entries(grouped).map(([step, messages]) => ({
			step: Number(step),
			messages
		}));
	}
	let substepMessages = $derived(
		groupByStepAsArray(stepMessages ?? new Map()).map((msg) =>
			msg.messages.filter(
				(m: MessageType) =>
					m.stepID?.includes(step.id) &&
					!m.sent &&
					m.message.length > 0 &&
					m.message.join('').trim() !== ''
			)
		)
	);
	let paginationArr = $state(step.loop?.map((_) => 0) ?? []);

	$effect(() => {
		if (parentShowOutput !== undefined) {
			showOutput = parentShowOutput;
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
		if (running || pending) {
			if (runID) {
				await ChatService.abort(project.assistantID, project.id, {
					taskID: task.id,
					runID: runID
				});
			}
			return;
		}
		if (running || pending || !step.step || step.step?.trim() === '') {
			return;
		}
		await run?.(step);
	}

	async function toggleLoop() {
		if (!isLoopStep) {
			step.loop = step.loop ?? [''];
			paginationArr.push(0);
		} else {
			step.loop = undefined;
			paginationArr = [];
		}
	}

	function removeSubStep(i: number) {
		step.loop!.splice(i, 1);
		paginationArr.splice(i, 1);
		substepMessages[i].forEach((msg) => {
			stepMessages?.delete(msg.stepID ?? '');
		});
	}
	function addSubStep(i: number) {
		step.loop!.splice(i + 1, 0, '');
		paginationArr.splice(i + 1, 0, 0);
		substepMessages.splice(i + 1, 0, []);
	}
</script>

{#snippet outputVisibilityButton()}
	<div class="size-10">
		{#if messages.length > 0}
			<button
				class="icon-button"
				data-testid="step-toggle-output-btn"
				onclick={() => (showOutput = !showOutput)}
				use:tooltip={'Toggle Output Visibility'}
				transition:fade={{ duration: 200 }}
			>
				{#if showOutput}
					<Eye class="size-4" />
				{:else}
					<EyeClosed class="size-4" />
				{/if}
			</button>
		{/if}
	</div>
{/snippet}

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
					<div
						class="relative my-3 -ml-4 flex min-h-[150px] flex-col gap-4 rounded-lg bg-white p-5 transition-transform dark:bg-black"
						class:border-2={running}
						class:border-blue={running}
						transition:slide
					>
						{#each loopDataMessages as msg}
							{#if !msg.sent}
								<Message {msg} {project} disableMessageToEditor />
							{/if}
						{/each}
					</div>
				{/if}
				<div class="flex flex-col gap-2 pl-6">
					{#each step.loop! as _, i}
						<div class="flex flex-col gap-2">
							<div class="flex items-center gap-2">
								{index + 1}.{i + 1}
								<textarea
									{onkeydown}
									rows="1"
									placeholder="Instructions..."
									use:autoHeight
									bind:value={step.loop![i]}
									class="ghost-input border-surface2 grow resize-none"
									disabled={readOnly}
								></textarea>
								{#if !readOnly}
									<div class="flex items-center">
										<button
											class="icon-button"
											onclick={() => removeSubStep(i)}
											use:tooltip={'Remove step from loop'}
										>
											<Trash2 class="size-4" />
										</button>
										{#if i === step.loop!.length - 1}
											<button
												class="icon-button self-start"
												onclick={() => addSubStep(i)}
												use:tooltip={'Add step to loop'}
											>
												<Plus class="size-4" />
											</button>
										{/if}
									</div>
								{/if}
							</div>
							{#if substepMessages[i]?.length > 0 && showOutput}
								<div
									class="relative my-3 -ml-4 flex min-h-[150px] flex-col gap-4 rounded-lg bg-white p-5 transition-transform dark:bg-black"
									class:border-2={running}
									class:border-blue={running}
									transition:slide
								>
									{#each substepMessages[i] as msg, index}
										{#if !msg.sent && paginationArr[i] === index}
											<Message {msg} {project} disableMessageToEditor maxHeight={'300px'} />
										{/if}
									{/each}
									<div
										class="absolute right-2 bottom-2 mb-2 flex items-center justify-end gap-1 p-2"
									>
										<button
											onclick={() =>
												paginationArr &&
												(paginationArr[i] =
													(paginationArr[i] - 1 + substepMessages[i].length) %
													substepMessages[i].length)}
											disabled={(paginationArr?.[i] ?? 0) === 0}
											class="rounded-md p-1 opacity-100 hover:bg-gray-300 disabled:opacity-50"
										>
											<ArrowLeft class="size-4" />
										</button>
										<select
											onchange={(e) => {
												if (e.target)
													(paginationArr ??= [])[i] = Number((e.target as HTMLSelectElement).value);
											}}
											class="flex appearance-none items-center justify-center border px-2 text-sm"
											value={paginationArr?.[i]}
										>
											{#each substepMessages[i] as _, index}
												<option value={index}>
													{index + 1}
												</option>
											{/each}
										</select>
										<button
											onclick={() =>
												((paginationArr ??= [])[i] =
													((paginationArr[i] ?? 0) + 1) % substepMessages[i].length)}
											disabled={(paginationArr?.[i] ?? 0) === substepMessages[i].length - 1}
											class="rounded-md p-1 opacity-100 hover:bg-gray-300 disabled:opacity-50"
										>
											<ArrowRight class="size-4" />
										</button>
									</div>
								</div>
							{/if}
						</div>
					{/each}
				</div>
			{/if}
		</div>
		<div class="flex shrink-0">
			{#if readOnly}
				{@render outputVisibilityButton()}
			{:else}
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
					use:tooltip={running
						? 'Abort'
						: pending
							? 'Running...'
							: messages.length > 0
								? 'Re-run Step'
								: 'Run Step'}
				>
					{#if running}
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
					{@render outputVisibilityButton()}
				</div>
				<div
					class="flex items-center justify-center rounded-full p-2 text-gray-500 hover:cursor-grab hover:bg-gray-200 active:cursor-grabbing"
				>
					<GripVertical class="size-6" />
				</div>
			{/if}
		</div>
	</div>
	{#if !isLoopStep && messages.length > 0}
		{#if showOutput}
			<div
				class="relative my-3 -ml-4 flex min-h-[150px] flex-col gap-4 rounded-lg bg-white p-5 transition-transform dark:bg-black"
				class:border-2={running}
				class:border-blue={running}
				transition:slide
			>
				{#each messages as msg}
					{#if !msg.sent}
						<Message {msg} {project} disableMessageToEditor />
					{/if}
				{/each}
				{#if stale}
					<div
						class="absolute inset-0 h-full w-full rounded-3xl bg-white opacity-80 dark:bg-black"
					></div>
				{/if}
			</div>
		{/if}
	{/if}
</li>

<Confirm
	show={toDelete !== undefined}
	msg={`Are you sure you want to delete this step`}
	onsuccess={deleteStep}
	oncancel={() => (toDelete = undefined)}
/>

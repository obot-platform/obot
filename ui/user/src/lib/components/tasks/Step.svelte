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
	import { Eye, EyeClosed, Plus, Trash2 } from 'lucide-svelte/icons';
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

	let messages = $derived(stepMessages?.get(step.id)?.messages ?? []);
	let running = $derived(stepMessages?.get(step.id)?.inProgress ?? false);
	let stale: boolean = $derived(parentStale || !parentMatches());
	let toDelete = $state<boolean>();
	let showOutput = $state(true);

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
	<div class="flex items-center justify-between gap-6">
		<textarea
			{onkeydown}
			rows="1"
			placeholder="Instructions..."
			use:autoHeight
			id={'step' + step.id}
			bind:value={step.step}
			class="ghost-input border-surface2 ml-1 grow resize-none"
			disabled={readOnly}
		></textarea>
		<div class="flex">
			{#if readOnly}
				{@render outputVisibilityButton()}
			{:else}
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
			{/if}
		</div>
	</div>
	{#if messages.length > 0}
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

{#if task.steps.length > index + 1}
	{#key task.steps[index + 1].id}
		<Self
			{run}
			{runID}
			{pending}
			{task}
			index={index + 1}
			bind:step={task.steps[index + 1]}
			{stepMessages}
			parentStale={stale}
			{project}
			showOutput={parentShowOutput}
			{readOnly}
		/>
	{/key}
{/if}

<Confirm
	show={toDelete !== undefined}
	msg={`Are you sure you want to delete this step`}
	onsuccess={deleteStep}
	oncancel={() => (toDelete = undefined)}
/>

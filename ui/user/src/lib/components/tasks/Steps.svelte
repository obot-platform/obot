<script lang="ts">
	import { type Messages, type Project, type Task, type TaskStep } from '$lib/services';
	import Step from '$lib/components/tasks/Step.svelte';
	import { SvelteMap } from 'svelte/reactivity';
	import Files from '$lib/components/tasks/Files.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { Eye, EyeClosed } from 'lucide-svelte';

	interface Props {
		task: Task;
		runID?: string;
		project: Project;
		run: (step?: TaskStep) => Promise<void>;
		stepMessages: SvelteMap<string, Messages>;
		pending: boolean;
		running: boolean;
		error: string;
		showAllOutput: boolean;
		readOnly?: boolean;
	}

	let {
		task = $bindable(),
		runID,
		showAllOutput = $bindable(),
		project,
		run,
		stepMessages,
		pending,
		running,
		error,
		readOnly
	}: Props = $props();

	// Drag, Drop

	let draggingIndex = $state<number | null>(null);
	let hoveredIndex = $state<number | null>(null);

	function handleDragStart(i: number) {
		draggingIndex = i;
	}

	function handleDragOver(e: DragEvent, i: number) {
		e.preventDefault();
		hoveredIndex = i;
		if (draggingIndex !== null && draggingIndex !== i) {
			hoveredIndex = i;
		}
	}

	function handleDrop() {
		if (draggingIndex !== null && hoveredIndex !== null && draggingIndex !== hoveredIndex) {
			const moved = task.steps[draggingIndex];
			task.steps.splice(draggingIndex, 1);
			task.steps.splice(hoveredIndex, 0, moved);
		}
		resetDragState();
	}

	function handleDragEnd() {
		resetDragState();
	}

	function resetDragState() {
		draggingIndex = null;
		hoveredIndex = null;
	}
</script>

<div class="rounded-lg bg-gray-50 p-5 dark:bg-gray-950">
	<div class="flex w-full items-center justify-between">
		<h4 class="text-lg font-semibold">Steps</h4>
		<button
			class="icon-button"
			data-testid="steps-toggle-output-btn"
			onclick={() => (showAllOutput = !showAllOutput)}
			use:tooltip={'Toggle All Output Visbility'}
		>
			{#if showAllOutput}
				<Eye class="size-5" />
			{:else}
				<EyeClosed class="size-5" />
			{/if}
		</button>
	</div>

	<ol class="list-decimal pt-2 opacity-100">
		{#if task.steps.length > 0}
			{#each task.steps as step, index (step.id)}
				<div
					class:drop-target={hoveredIndex === index && draggingIndex !== index}
					class:dragging={draggingIndex === index}
					class:dragging-over={hoveredIndex === index}
					class:dragging-into={draggingIndex !== null && draggingIndex !== index}
					draggable={!readOnly}
					role="listitem"
					ondragstart={() => handleDragStart(index)}
					ondragover={(e) => handleDragOver(e, index)}
					ondrop={() => handleDrop()}
					ondragend={handleDragEnd}
				>
					<Step
						{run}
						{runID}
						bind:task
						bind:step={task.steps[index]}
						{index}
						{stepMessages}
						{pending}
						{project}
						showOutput={showAllOutput}
						{readOnly}
					/>
				</div>
			{/each}
		{/if}
	</ol>

	{#if error}
		<div class="mt-2 text-red-500">{error}</div>
	{/if}
</div>

{#if runID}
	<Files taskID={task.id} {runID} running={running || pending} {project} />
{/if}

<style>
	.drop-target {
		background-color: var(--color-gray-100);
	}
	.dragging {
		background-color: var(--color-gray-200);
	}
	.dragging-over {
		background-color: var(--color-gray-300);
	}
</style>

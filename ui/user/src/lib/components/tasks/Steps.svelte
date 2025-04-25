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
</script>

<div class="dark:bg-surface1 dark:border-surface3 rounded-lg bg-white p-5 shadow-sm dark:border">
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
			{#key task.steps[0].id}
				<Step
					{run}
					{runID}
					bind:task
					bind:step={task.steps[0]}
					index={0}
					{stepMessages}
					{pending}
					{project}
					showOutput={showAllOutput}
					{readOnly}
				/>
			{/key}
		{/if}
	</ol>

	{#if error}
		<div class="mt-2 text-red-500">{error}</div>
	{/if}
</div>

{#if runID}
	<Files taskID={task.id} {runID} running={running || pending} {project} />
{/if}

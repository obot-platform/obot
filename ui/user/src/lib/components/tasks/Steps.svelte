<script lang="ts">
	import {
		ChatService,
		type Messages,
		type Project,
		type Task,
		type TaskStep
	} from '$lib/services';
	import { onDestroy } from 'svelte';
	import Step from '$lib/components/tasks/Step.svelte';
	import { LoaderCircle, OctagonX, Play } from 'lucide-svelte';
	import { SvelteMap } from 'svelte/reactivity';
	import { Thread } from '$lib/services/chat/thread.svelte';
	import Input from '$lib/components/tasks/Input.svelte';
	import Type from '$lib/components/tasks/Type.svelte';
	import Files from '$lib/components/tasks/Files.svelte';
	import { errors } from '$lib/stores';
	import type { EditorItem } from '$lib/services/editor/index.svelte';

	interface Props {
		task: Task;
		editMode?: boolean;
		onChanged?: (task: Task) => void | Promise<void>;
		save?: (steps: TaskStep[]) => void | Promise<void>;
		selectedRun?: string;
		project: Project;
		items: EditorItem[];
	}

	let { task, editMode = false, save, onChanged, selectedRun, project, items }: Props = $props();

	let stepMessages = new SvelteMap<string, Messages>();
	let allMessages = $state<Messages>({ messages: [], inProgress: false });
	let input = $state('');
	let error = $state('');
	let thread: Thread | undefined = $state<Thread>();
	let pending = $derived(thread?.pending ?? false);
	let running = $derived(allMessages.inProgress);
	let nextStep: Step | undefined = $state();
	let displayedRun = $state<string>('');

	onDestroy(closeThread);
	$effect(resetThread);

	function resetThread() {
		if (editMode) {
			if (displayedRun) {
				closeThread();
			}
			if (!thread) {
				newThread();
			}
		} else {
			if (selectedRun) {
				if (displayedRun !== selectedRun) {
					newThread(selectedRun);
				}
			} else {
				closeThread();
			}
		}
		error = '';
	}

	function closeThread() {
		if (!thread) {
			return;
		}

		thread.close();
		thread = undefined;
		stepMessages.clear();
		allMessages = { messages: [], inProgress: false };
		displayedRun = '';
	}

	function newThread(runID?: string) {
		closeThread();
		thread = new Thread(project, {
			onError: errors.items.push,
			task: task,
			runID: runID
		});
		stepMessages.clear();
		thread.onStepMessages = (stepID, messages) => {
			stepMessages.set(stepID, messages);
		};
		thread.onMessages = (messages) => {
			allMessages = messages;
		};
		displayedRun = runID || '';
	}

	async function click() {
		error = '';

		const hasAtLeastOneInstruction = task.steps.some((step) => (step.step ?? '').trim().length > 0);
		if (!hasAtLeastOneInstruction) {
			error = 'At least one instruction is required to run the task.';
			return;
		}

		if (running || pending) {
			return await ChatService.abort(project.assistantID, project.id, {
				taskID: task.id,
				runID: 'editor'
			});
		}
		if (nextStep) {
			await nextStep.saveAll();
		}
		await run();
	}

	async function run(step?: TaskStep, saveSteps?: TaskStep[]) {
		error = '';
		if (!thread || !task.id) {
			return;
		}

		if (saveSteps) {
			await save?.(saveSteps);
		}

		await thread.runTask(task.id, {
			stepID: step?.id || '*',
			input: input
		});
	}
</script>

<Input {editMode} bind:input {task} displayRunID={selectedRun} {project} />

<div class="mt-8 rounded-3xl bg-gray-50 p-5 dark:bg-gray-950">
	<div class="flex items-center justify-between">
		<h4 class="text-xl font-semibold">Steps</h4>
		<Type {task} {editMode} {onChanged} />
		{#if editMode}
			<div>
				<button
					class="ml-2 flex items-center gap-2 rounded-3xl bg-blue px-5 py-2 text-white hover:bg-blue-400"
					onclick={click}
				>
					{#if running}
						Stop
						<OctagonX class="h-4 w-4" />
					{:else if pending}
						Cancel
						<LoaderCircle class="h-4 w-4 animate-spin" />
					{:else}
						Test <Play class="h-4 w-4" />
					{/if}
				</button>
			</div>
		{/if}
	</div>

	<ol class="list-decimal pt-2 opacity-100">
		{#if task.steps.length > 0}
			{#key task.steps[0].id}
				<Step
					bind:this={nextStep}
					{run}
					onChange={save}
					{task}
					index={0}
					{stepMessages}
					{editMode}
					{pending}
					{project}
				/>
			{/key}
		{/if}
	</ol>

	{#if error}
		<div class="mt-2 text-red-500">{error}</div>
	{/if}
</div>

{#if selectedRun}
	<Files taskID={task.id} runID={selectedRun} running={running || pending} {project} {items} />
{:else if editMode}
	<Files taskID={task.id} runID="editor" running={running || pending} {project} {items} />
{/if}

<script lang="ts">
	import { ChatService, EditorService, type Project, type Task } from '$lib/services';
	import { Trash } from 'lucide-svelte/icons';
	import { Pen, PenOff } from 'lucide-svelte';
	import { onDestroy } from 'svelte';
	import Runs from '$lib/components/tasks/Runs.svelte';
	import Trigger from '$lib/components/tasks/Trigger.svelte';
	import Steps from '$lib/components/tasks/Steps.svelte';
	import Controls from '$lib/components/editor/Controls.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import type { EditorItem } from '$lib/services/editor/index.svelte';

	interface Props {
		id: string;
		project: Project;
		items: EditorItem[];
		onChanged?: (task: Task) => void | Promise<void>;
	}

	let { id, onChanged, project, items = $bindable() }: Props = $props();
	let savedTask = $state<string>();
	let selectedRun = $state<string>('');
	let displayedRun = $state<string>('');
	let task = $state<Task>({
		name: 'Loading...',
		steps: [],
		id: ''
	});
	let editMode = $state(false);

	$effect(() => {
		if (editMode) {
			selectedRun = '';
		}
	});

	$effect(() => {
		if (selectedRun) {
			if (editMode) {
				selectedRun = '';
				return;
			}
			if (displayedRun === selectedRun) {
				return;
			}
			ChatService.getTaskRun(project.assistantID, project.id, id, selectedRun).then((run) => {
				task = {
					...task,
					steps: run.task.steps
				};
			});
			displayedRun = selectedRun;
			savedTask = '';
			return;
		}

		displayedRun = '';

		if (!savedTask) {
			ChatService.getTask(project.assistantID, project.id, id).then((newTask) => {
				savedTask = JSON.stringify(newTask);
				task = newTask;
			});
		}
	});

	$effect(() => {
		if (task.id && editMode && task.steps.length === 0) {
			task.steps.push({
				id: 'si1' + Math.random().toString(36).substring(6)
			});
		}
		if (task.id && !editMode && task.steps.length === 0) {
			editMode = true;
		}
	});

	function isDirty() {
		return savedTask !== JSON.stringify(task);
	}

	onDestroy(() => {
		if (inflight) {
			clearTimeout(inflight);
		}
	});

	let inflight: number = 0;
	let toDelete: boolean = $state(false);

	function saveLater() {
		if (inflight) {
			clearTimeout(inflight);
			inflight = 0;
		}
		inflight = setTimeout(() => {
			inflight = 0;
			save();
		}, 1000);
	}

	async function deleteTask() {
		toDelete = false;
		await ChatService.deleteTask(project.assistantID, project.id, id);
		EditorService.remove(items, id);
	}

	async function saveOnEnter(e: KeyboardEvent) {
		if (e.target instanceof HTMLElement && e.key === 'Enter') {
			await save();
			e.target.blur();
		}
	}

	async function save() {
		if (!task || !isDirty()) {
			return;
		}

		task = await ChatService.saveTask(project.assistantID, project.id, task);
		savedTask = JSON.stringify(task);
		onChanged?.(task);
	}
</script>

<div
	role="none"
	onkeydown={(e) => e.stopPropagation()}
	class="relative flex flex-col rounded-s-3xl p-5"
>
	<input
		class="bg-white text-xl font-semibold outline-none dark:bg-black dark:text-gray-50"
		bind:value={task.name}
		onfocusout={saveLater}
		disabled={!editMode}
		onkeydown={saveOnEnter}
	/>

	<input
		class="mt-0.5 bg-white text-sm outline-none dark:bg-black dark:text-gray-50"
		bind:value={task.description}
		placeholder={editMode ? 'Enter description' : ''}
		onfocusout={saveLater}
		disabled={!editMode}
		onkeydown={saveOnEnter}
	/>

	<div class="overflow-auto">
		{#if !editMode}
			<Runs
				{project}
				{id}
				onSelect={(i) => {
					selectedRun = i;
				}}
			/>
		{/if}

		<Trigger
			{task}
			{editMode}
			onChanged={async (t) => {
				task = t;
				await save();
			}}
		/>

		<Steps
			{task}
			{editMode}
			{selectedRun}
			{project}
			{items}
			onChanged={async (t) => {
				task = t;
				await save();
			}}
			save={async (steps) => {
				task.steps = steps;
				await save();
			}}
		/>
	</div>

	<div class="absolute right-0 top-0 m-2 flex">
		<button
			class="icon-button"
			onclick={() => {
				editMode = !editMode;
			}}
		>
			{#if editMode}
				<PenOff class="h-5 w-5" />
			{:else}
				<Pen class="h-5 w-5" />
			{/if}
		</button>
		<button class="icon-button" onclick={() => (toDelete = true)}>
			<Trash class="h-5 w-5" />
		</button>
		<Controls {project} {items} />
	</div>

	<div class="m-2 grow place-content-end self-end text-gray-300">id: {id}</div>
</div>

<Confirm
	show={toDelete}
	msg={`Are you sure you want to delete this task`}
	onsuccess={deleteTask}
	oncancel={() => (toDelete = false)}
/>

<style lang="postcss">
</style>

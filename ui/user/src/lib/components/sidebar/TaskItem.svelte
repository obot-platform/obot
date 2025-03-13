<script lang="ts">
	import { ChevronDown, Pencil, Trash2 } from 'lucide-svelte/icons';
	import { overflowToolTip } from '$lib/actions/overflow';
	import DotDotDot from '../DotDotDot.svelte';
	import { getLayout } from '$lib/context/layout.svelte';
	import { type Task, type Thread } from '$lib/services';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		task: Task;
		onDelete?: (task: Task) => void;
		taskRuns?: Thread[];
		currentThreadID?: string;
		expanded?: boolean;
	}

	let {
		task,
		taskRuns,
		onDelete,
		currentThreadID = $bindable(),
		expanded: initialExpanded
	}: Props = $props();
	const layout = getLayout();

	let expanded = $state(initialExpanded ?? false);
</script>

<li class="group flex min-h-9 flex-col">
	<div class="flex items-center gap-3 rounded-md p-2">
		<div class="flex grow items-center gap-1">
			{#if taskRuns && taskRuns.length > 0}
				<button onclick={() => (expanded = !expanded)}>
					<ChevronDown
						class={twMerge('size-4 transition-transform duration-200', expanded && 'rotate-180')}
					/>
				</button>
			{/if}
			<div
				use:overflowToolTip
				class:font-normal={layout.editTaskID === task.id}
				class="flex flex-1 grow items-center text-xs font-light"
			>
				{task.name ?? ''}
			</div>
		</div>
		<DotDotDot class="p-0 opacity-0 transition-opacity duration-200 group-hover:opacity-100">
			<div class="default-dialog flex min-w-40 flex-col p-2">
				<button
					class="menu-button"
					onclick={async () => {
						layout.editTaskID = task.id;
					}}
				>
					<Pencil class="size-4" /> Edit Task
				</button>
				<button class="menu-button" onclick={() => onDelete?.(task)}>
					<Trash2 class="size-4" /> Delete
				</button>
			</div>
		</DotDotDot>
	</div>
	{#if expanded && taskRuns && taskRuns?.length > 0}
		<ul class="flex flex-col pl-5 text-xs">
			{#each taskRuns as taskRun}
				<li class:bg-surface2={currentThreadID === taskRun.id} class="w-full">
					<button
						class="flex w-full justify-between rounded-md p-2 text-left hover:bg-surface3"
						onclick={() => {
							layout.editTaskID = undefined;
							currentThreadID = taskRun.id;
						}}
					>
						<span
							>{new Date(taskRun.created)
								.toLocaleString('en-US', {
									year: 'numeric',
									month: '2-digit',
									day: '2-digit'
								})
								.replace(/\//g, '-')}
						</span>
						<span>
							{new Date(taskRun.created)
								.toLocaleString('en-US', {
									hour: '2-digit',
									minute: '2-digit',
									hour12: true
								})
								.replace(/\//g, '-')}
						</span>
					</button>
				</li>
			{/each}
		</ul>
	{/if}
</li>

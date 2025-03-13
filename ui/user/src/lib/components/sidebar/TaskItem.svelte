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
	let displayCount = $state(10); // number of task runs to display initially

	function loadMore() {
		displayCount += 10;
	}
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
			{#each taskRuns.slice(0, displayCount) as taskRun}
				<li class:bg-surface2={currentThreadID === taskRun.id} class="w-full">
					<button
						class="w-full rounded-md p-2 text-left hover:bg-surface3"
						onclick={() => {
							layout.editTaskID = undefined;
							currentThreadID = taskRun.id;
						}}
					>
						{new Date(taskRun.created)
							.toLocaleString(undefined, {
								year: 'numeric',
								month: '2-digit',
								day: '2-digit',
								hour: 'numeric',
								minute: '2-digit',
								hour12: true
							})
							.replace(/\//g, '-')
							.replace(/,/g, '')}
					</button>
				</li>
			{/each}
			{#if taskRuns?.length && taskRuns?.length > displayCount}
				<li class="flex w-full justify-center rounded-md p-2 hover:bg-surface3">
					<button class="w-full text-xs" onclick={loadMore}> Show More </button>
				</li>
			{/if}
		</ul>
	{/if}
</li>

<script lang="ts">
	import { ChevronDown, Pencil, Trash2 } from 'lucide-svelte/icons';
	import { overflowToolTip } from '$lib/actions/overflow';
	import DotDotDot from '../DotDotDot.svelte';
	import { closeAll, getLayout, isSomethingSelected, openTask } from '$lib/context/layout.svelte';
	import { formatTime } from '$lib/time.js';
	import { type Task, type Thread, type Project } from '$lib/services';
	import { twMerge } from 'tailwind-merge';
	import { ChatService } from '$lib/services';
	import { responsive } from '$lib/stores';

	interface Props {
		task: Task;
		onDelete?: (task: Task) => void;
		taskRuns?: Thread[];
		currentThreadID?: string;
		expanded?: boolean;
		project: Project;
	}

	let {
		task,
		taskRuns,
		onDelete,
		currentThreadID = $bindable(),
		expanded: initialExpanded,
		project
	}: Props = $props();
	const layout = getLayout();

	let expanded = $state(initialExpanded ?? false);
	let displayCount = $state(10); // number of task runs to display initially

	function loadMore() {
		displayCount += 10;
	}

	async function deleteTaskRun(taskRun: Thread) {
		if (!taskRun.id || !task.id) return;

		await ChatService.deleteTaskRun(project.assistantID, project.id, task.id, taskRun.taskRunID!);

		// Update the local list to remove the deleted run
		if (taskRuns) {
			taskRuns = taskRuns.filter((run) => run.id !== taskRun.id);
		}

		// If this was the current thread, select another one
		if (currentThreadID === taskRun.id) {
			currentThreadID = '';
			layout.items = [];
		}
	}
</script>

<li class="flex min-h-9 flex-col">
	<div class="group flex items-center gap-3 rounded-md p-2 hover:bg-surface3">
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
		<DotDotDot class="p-0 transition-opacity duration-200 group-hover:opacity-100 md:opacity-0">
			<div class="default-dialog flex min-w-40 flex-col p-2">
				<button
					class="menu-button"
					onclick={async () => {
						if (responsive.isMobile) {
							layout.sidebarOpen = false;
						}
						openTask(layout, task.id);
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
				<li
					class:bg-surface2={currentThreadID === taskRun.id && !isSomethingSelected(layout)}
					class="w-full"
				>
					<div class="group flex items-center rounded-md hover:bg-surface3">
						<button
							class="w-full p-2 text-left"
							onclick={() => {
								closeAll(layout);
								currentThreadID = taskRun.id;
							}}
						>
							{formatTime(taskRun.created)}
						</button>
						<DotDotDot
							class="p-0 pr-2 opacity-0 transition-opacity duration-200 group-hover:opacity-100"
						>
							<div class="default-dialog flex min-w-max flex-col p-2">
								<button class="menu-button" onclick={() => deleteTaskRun(taskRun)}>
									<Trash2 class="h-4 w-4" /> Delete
								</button>
							</div>
						</DotDotDot>
					</div>
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

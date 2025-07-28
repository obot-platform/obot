<script lang="ts">
	import { ChevronDown, Trash2 } from 'lucide-svelte/icons';
	import { overflowToolTip } from '$lib/actions/overflow';
	import {
		getLayout,
		isSomethingSelected,
		openTask,
		openTaskRun
	} from '$lib/context/chatLayout.svelte';
	import { formatTime } from '$lib/time.js';
	import { type Task, type Thread, type Project } from '$lib/services';
	import { twMerge } from 'tailwind-merge';
	import { ChatService } from '$lib/services';
	import { responsive } from '$lib/stores';
	import type { Snippet } from 'svelte';
	import { slide } from 'svelte/transition';
	import { tooltip } from '$lib/actions/tooltip.svelte';

	interface Props {
		task: Task;
		taskRuns?: Thread[];
		currentThreadID?: string;
		expanded?: boolean;
		project: Project;
		taskActions?: Snippet;
		classes?: {
			title?: string;
			taskItemAction?: string;
		};
	}

	let {
		task,
		taskRuns,
		currentThreadID = $bindable(),
		expanded: initialExpanded,
		project,
		taskActions,
		classes
	}: Props = $props();
	const layout = getLayout();

	let expanded = $state((taskRuns?.length ?? 0) > 0 && initialExpanded ? true : false);
	let displayCount = $state(10); // number of task runs to display initially

	function loadMore() {
		displayCount += 10;
	}

	function isTaskFromIntegration(task: Task) {
		return (
			task.id === project.workflowNamesFromIntegration?.slackWorkflowName ||
			task.id === project.workflowNamesFromIntegration?.discordWorkflowName ||
			task.id === project.workflowNamesFromIntegration?.emailWorkflowName ||
			task.id === project.workflowNamesFromIntegration?.webhookWorkflowName
		);
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
	<div
		class={twMerge(
			'hover:bg-surface3/90 active:bg-surface3/100 group mb-[2px] flex items-center rounded-md font-light transition-colors duration-200',
			layout.editTaskID === task.id && 'bg-surface3/60',
			layout.displayTaskRun && layout.displayTaskRun.taskID === task.id && 'font-medium'
		)}
	>
		<div class="flex grow items-center gap-1 truncate pl-1.5">
			<button class="p-1" onclick={() => (expanded = !expanded)}>
				<ChevronDown
					class={twMerge('size-4 transition-transform duration-200', expanded && 'rotate-180')}
				/>
			</button>
			<button
				use:overflowToolTip
				class:font-medium={layout.editTaskID === task.id}
				class={twMerge('grow py-2 pr-2 pl-1 text-left text-xs', classes?.title)}
				onclick={async () => {
					if (responsive.isMobile) {
						layout.sidebarOpen = false;
					}
					if (layout.editTaskID === task.id && expanded) {
						expanded = false;
					} else {
						expanded = true;
						openTask(layout, task.id);
					}
				}}
			>
				{task.name ?? ''}
			</button>
		</div>
		{#if taskActions}
			{@render taskActions()}
		{/if}
	</div>
	{#if expanded}
		<ul class="ml-4 flex flex-col text-xs" transition:slide>
			{#if taskRuns && taskRuns?.length > 0}
				{#each taskRuns?.slice(0, displayCount) ?? [] as taskRun (taskRun.id)}
					<li class="track-mark relative w-full pb-[2px] pl-3">
						<div
							class={twMerge(
								'hover:bg-surface3/90 active:bg-surface3/100 group flex items-center rounded-md transition-colors duration-200',
								currentThreadID === taskRun.id && !isSomethingSelected(layout) && 'bg-surface2',
								layout.displayTaskRun &&
									layout.displayTaskRun.id === taskRun.taskRunID &&
									'bg-surface3/60'
							)}
						>
							<button
								class="w-full p-2 text-left"
								onclick={async () => {
									if (taskRun.taskID && taskRun.taskRunID) {
										openTaskRun(
											layout,
											await ChatService.getTaskRun(
												project.assistantID,
												project.id,
												taskRun.taskID,
												taskRun.taskRunID
											)
										);

										if (responsive.isMobile) {
											layout.sidebarOpen = false;
										}
									}
								}}
							>
								{formatTime(taskRun.created)}
							</button>
							{#if !isTaskFromIntegration(task)}
								<button
									class={twMerge(
										'p-0 pr-4 opacity-0 transition-opacity duration-200 group-hover:opacity-100',
										classes?.taskItemAction
									)}
									onclick={() => deleteTaskRun(taskRun)}
									use:tooltip={'Delete Run'}
								>
									<Trash2 class="size-4" />
								</button>
							{/if}
						</div>
					</li>
				{/each}
				{#if taskRuns?.length && taskRuns?.length > displayCount}
					<li class="hover:bg-surface3 flex w-full justify-center rounded-md p-2">
						<button class="w-full text-xs" onclick={loadMore}> Show More </button>
					</li>
				{/if}
			{:else}
				<li
					class="track-mark text-gray relative flex w-full justify-start rounded-md p-2 pl-5 font-light"
				>
					<p class="text-xs">No task runs</p>
				</li>
			{/if}
		</ul>
	{/if}
</li>

<style lang="postcss">
	.track-mark::after {
		content: '';
		position: absolute;
		top: 50%;
		transform: translateY(-50%);
		left: 0;
		width: 12px;
		height: 1px;
		background-color: var(--surface3);
	}
	.track-mark::before {
		content: '';
		position: absolute;
		top: 0;
		left: 0;
		height: 100%;
		width: 1px;
		background-color: var(--surface3);
	}
	.track-mark:last-child::before {
		content: '';
		position: absolute;
		top: 0;
		left: 0;
		height: 50%;
		width: 1px;
		background-color: var(--surface3);
	}
</style>

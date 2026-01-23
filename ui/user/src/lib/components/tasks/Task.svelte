<script lang="ts">
	import {
		ChatService,
		getTaskRun,
		type Messages,
		type Project,
		type Task,
		type TaskStep,
		type TaskRun
	} from '$lib/services';
	import { MessageCircle, MessageCircleOff, Trash2, TriangleAlert } from 'lucide-svelte/icons';
	import { onDestroy, onMount, untrack } from 'svelte';
	import Steps from '$lib/components/tasks/Steps.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import { newSaveMonitor } from '$lib/save.js';
	import { LoaderCircle, OctagonX, Play } from 'lucide-svelte';
	import { Thread } from '$lib/services/chat/thread.svelte';
	import { errors, responsive } from '$lib/stores';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { fade, slide } from 'svelte/transition';
	import TaskOptions from './TaskOptions.svelte';
	import { twMerge } from 'tailwind-merge';
	import ChatInput from '../messages/Input.svelte';
	import Input from './Input.svelte';
	import { getLayout } from '$lib/context/chatLayout.svelte';
	import { TASK_NEW_ID } from '$lib/constants';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';

	interface Props {
		task: Task;
		project: Project;
		onChanged?: (task: Task) => void | Promise<void>;
		onDelete?: () => void | Promise<void>;
		runID?: string;
		readonly?: boolean;
		skipFetchOnMount?: boolean;
		noChat?: boolean;
	}

	let {
		task = $bindable(),
		onChanged,
		project,
		onDelete,
		runID: inputRunID,
		readonly,
		skipFetchOnMount,
		noChat
	}: Props = $props();

	const readOnly = untrack(() => !!inputRunID || readonly);
	let runID = $state(untrack(() => inputRunID));
	let thread: Thread | undefined = $state<Thread>();
	let lastStepId: string | undefined = $state(undefined);
	let stepMessages: Record<string, Messages> = $state({});
	let allMessages = $state<Messages>({ messages: [], inProgress: false });
	let input = $state('');
	let error = $state('');
	let pending = $derived(thread?.pending ?? false);
	// let running = $derived(allMessages.inProgress);
	let inputDialog = $state<ReturnType<typeof ResponsiveDialog>>();

	let showChat = $state(false);
	let showAllOutput = $state(true);

	let taskHeaderActionDiv: HTMLDivElement | undefined = $state<HTMLDivElement>();
	let isTaskInfoVisible = $state(true);
	let observer: IntersectionObserver;

	let taskRun: Task | undefined = $state(undefined);
	let taskRunData: TaskRun | undefined = $state(undefined);
	const layout = getLayout();

	$effect(() => {
		// Load task run data only in readonly mode; so that we can get the correct steps & loop steps related to the current task
		if (readOnly && project && runID) {
			getTaskRun(project.assistantID, project.id, task.id, runID).then((res) => {
				taskRunData = res;
				taskRun = res.task;
			});
		}
	});

	// TODO: Find a way to auto close the thread when task run is completed

	/**************************************************************************************************/
	// HACK: fix glitch that happens when in messages.inProgress when loop steps executed
	// Make sure that the .inProgress stays true until the end of the current run task

	let timeoutId: number | undefined = undefined;
	// save how many step.inProgress === false we got

	let isRunning = $state(false);

	// Indicate whther the user is activating the task run following or not
	let shouldFollowTaskRun = $state(true);

	// when not in progess is recieved; wait 1 seconds and force set task as not running
	$effect(() => {
		clearTimeout(timeoutId);
		if (!allMessages?.inProgress) {
			untrack(() => {
				timeoutId = setTimeout(() => {
					isRunning = false;
					shouldFollowTaskRun = false;
				}, 1000);
			});
		}
	});

	let creatingTask = false;

	function isNewTask(taskId: string): boolean {
		return taskId.startsWith(TASK_NEW_ID);
	}

	const saver = newSaveMonitor(
		() => task,
		async (t: Task) => {
			if (isNewTask(task.id)) {
				if (creatingTask) {
					// Creation in progress, skip this save - it will be retried on the next interval
					return t;
				}
				creatingTask = true;
				try {
					const result = await ChatService.createTask(project.assistantID, project.id, t);
					task = { ...task, id: result.id };
					layout.editTaskID = result.id;
					return result;
				} catch (e) {
					creatingTask = false;
					throw e;
				}
			}
			return await ChatService.saveTask(project.assistantID, project.id, t);
		},
		(t) => {
			task = t;
			onChanged?.(t);
		}
	);

	$effect(resetThread);

	let toDelete: boolean = $state(false);

	async function deleteTask() {
		if (readOnly) {
			return;
		}

		if (!isNewTask(task.id)) {
			await ChatService.deleteTask(project.assistantID, project.id, task.id);
		}

		onDelete?.();
		toDelete = false;
	}

	function setupObserver() {
		// Always disconnect existing observer before setting up new one
		observer?.disconnect();

		observer = new IntersectionObserver(
			([entry]) => {
				isTaskInfoVisible = entry.isIntersecting;
			},
			{ threshold: 0 }
		);

		if (taskHeaderActionDiv) {
			observer.observe(taskHeaderActionDiv);
		}
	}
	onDestroy(() => {
		if (!readOnly) {
			saver.stop();
		}
		closeThread();
	});

	$effect(() => {
		if (taskHeaderActionDiv) {
			setupObserver();
		}
	});

	onMount(async () => {
		if (!skipFetchOnMount && !isNewTask(task.id)) {
			task = await ChatService.getTask(project.assistantID, project.id, task.id);
		}
		if (!readOnly) {
			saver.start();
		}

		if (taskHeaderActionDiv) {
			setupObserver();
		}
	});

	onMount(() => {
		setupObserver();
		return () => observer.disconnect();
	});

	function resetThread() {
		if (!thread && runID) {
			newThread();
		}
		error = '';
	}

	function closeThread() {
		if (!thread) {
			return;
		}

		thread.close();
		thread = undefined;
		runID = undefined;
		lastStepId = undefined;
		stepMessages = {};
		allMessages = { messages: [], inProgress: false };
	}

	function newThread() {
		closeThread();
		thread = new Thread(project, {
			onError: errors.items.push,
			task: task,
			runID: runID
		});
		stepMessages = {};
		thread.onStepMessages = (stepID, messages) => {
			lastStepId = stepID;
			stepMessages[stepID] = messages;
		};
		thread.onMessages = (messages) => {
			allMessages = messages;
		};
	}

	async function click() {
		error = '';

		const hasAtLeastOneInstruction = task.steps.some((step) => (step.step ?? '').trim().length > 0);
		if (!hasAtLeastOneInstruction) {
			error = 'At least one instruction is required to run the task.';
			return;
		}

		if (isRunning || pending) {
			if (runID) {
				return await ChatService.abort(project.assistantID, project.id, {
					taskID: task.id,
					runID: runID
				});
			}

			isRunning = false;
			shouldFollowTaskRun = false;
			return;
		}

		if (task.onDemand) {
			inputDialog?.open();
			return;
		}

		try {
			// make sure this runs after run() function and before promise is resolved
			setTimeout(() => {
				isRunning = true;
				shouldFollowTaskRun = true;
			}, 0);

			await run();
		} catch (_) {
			isRunning = false;
		}
	}

	async function run(step?: TaskStep) {
		await saver.save();

		if (isRunning || pending) {
			return;
		}

		if (!step || !runID || !thread) {
			if (thread && (isRunning || pending)) {
				await thread.abort();
			}
			closeThread();
			runID = (
				await ChatService.runTask(project.assistantID, project.id, task.id, {
					stepID: step?.id,
					input
				})
			).id;
			return;
		}

		lastStepId = undefined;

		await thread.runStep(task.id, step.id, {
			input: input
		});
	}
</script>

{#snippet mainActions()}
	<div class="flex items-center gap-2">
		{#if allMessages.messages.length > 0 && !noChat}
			<button
				class="icon-button"
				onclick={() => (showChat = !showChat)}
				use:tooltip={'Toggle Chat'}
				transition:fade
			>
				{#if showChat}
					<MessageCircleOff class="size-6" />
				{:else}
					<MessageCircle class="size-6" />
				{/if}
			</button>
		{/if}
		{#if !readOnly}
			<button
				class="bg-primary hover:bg-primary/50 flex items-center justify-center gap-2 rounded-2xl px-12 py-2 text-white transition-all duration-200"
				onclick={click}
				class:grow={responsive.isMobile}
			>
				{#if isRunning}
					Stop
					<OctagonX class="h-4 w-4" />
				{:else if pending}
					Cancel
					<LoaderCircle class="h-4 w-4 animate-spin" />
				{:else}
					Run
					<Play class="h-4 w-4" />
				{/if}
			</button>
		{/if}
	</div>
{/snippet}

<div class="flex h-full w-full grow flex-col">
	<div
		class="sticky top-0 left-0 z-40 flex h-0 flex-col items-center justify-center px-4 opacity-0 transition-all duration-200 md:px-8"
		class:opacity-100={!isTaskInfoVisible}
		class:h-16={!isTaskInfoVisible}
	>
		<div class="flex h-16 w-full items-center justify-between gap-8 md:max-w-[1200px]">
			<h4 class="border-primary grow truncate border-l-4 pl-2 text-lg font-semibold md:text-xl">
				{task.name}
			</h4>
			{@render mainActions()}
		</div>
	</div>

	<div
		class={twMerge(
			'default-scrollbar-thin scrollbar-gutter-stable flex w-full grow justify-center overflow-y-auto px-4 md:px-8'
		)}
		data-scrollable="true"
	>
		<!-- div in div is needed for the scrollbar to work so that space outside the max-width is still scrollable -->
		<div
			role="none"
			onkeydown={(e) => e.stopPropagation()}
			class="relative flex w-full flex-col gap-4"
		>
			<div class="w-full self-center md:max-w-[1200px]">
				<div class="mt-8 mb-4 flex w-full justify-between gap-8 pb-0">
					<div class="border-primary flex grow flex-col gap-1 border-l-4 pl-4">
						<strong class="text-primary text-xs">TASK</strong>

						{#if readOnly}
							<h1 class="my-2 border-b border-transparent text-2xl font-semibold">{task.name}</h1>
						{:else}
							<input class="ghost-input text-2xl font-semibold" bind:value={task.name} />
						{/if}

						{#if readOnly}
							{#if task.description}
								<p class="text-gray py-2 text-base dark:text-gray-300">
									{task.description}
								</p>
							{/if}
						{:else}
							<input
								class="ghost-input"
								bind:value={task.description}
								placeholder="Enter description..."
							/>
						{/if}
					</div>

					{#if !responsive.isMobile}
						<div
							bind:this={taskHeaderActionDiv}
							class="flex h-full items-center justify-center gap-4 md:justify-between"
						>
							{@render mainActions()}
							{#if !readOnly}
								<button
									in:slide={{ axis: 'x', duration: 150 }}
									class="button-destructive p-3"
									onclick={() => (toDelete = true)}
								>
									<Trash2 class="size-4" />
								</button>
							{/if}
						</div>
					{/if}
				</div>
				{#if responsive.isMobile}
					<div bind:this={taskHeaderActionDiv} class="flex w-full justify-between px-4">
						<div class="flex">
							{@render mainActions()}
						</div>
						{#if !readOnly}
							<button class="button-destructive p-4" onclick={() => (toDelete = true)}>
								<Trash2 class="size-4" />
							</button>
						{/if}
					</div>
				{/if}
			</div>
			{#if taskRunData?.warning}
				<div class="notification-alert flex w-full flex-col gap-2">
					<div class="flex items-center gap-2">
						<TriangleAlert class="size-6 flex-shrink-0 self-start text-yellow-500" />
						<p class="my-0.5 flex flex-col text-sm font-semibold">Warning</p>
					</div>
					<span class="text-sm font-light break-all">{taskRunData.warning}</span>
				</div>
			{/if}
			<div class="flex w-full justify-center">
				<div
					class="bg-surface1 dark:bg-background flex w-full flex-col gap-4 rounded-xl p-4 shadow-inner md:max-w-[1200px]"
				>
					<div class="flex flex-col gap-4">
						<TaskOptions bind:task {readOnly} />

						<Steps
							bind:task
							bind:showAllOutput
							bind:shouldFollowTaskRun
							{taskRun}
							{lastStepId}
							{project}
							{run}
							{runID}
							{stepMessages}
							{pending}
							isTaskRunning={isRunning}
							{error}
							{readOnly}
						/>
					</div>
				</div>
			</div>

			<div class="grow"></div>

			<div
				class="bg-background sticky bottom-0 z-50 flex items-center justify-center px-6 opacity-0 transition-opacity"
				class:chat-overlay={showChat}
			>
				{#if allMessages.messages.length > 0 && showChat}
					<div
						transition:slide
						class="flex max-w-[1200px] grow items-center justify-center gap-4 py-4 md:max-w-full"
					>
						<ChatInput
							readonly={allMessages.inProgress}
							pending={thread?.pending}
							onAbort={async () => {
								await thread?.abort();
							}}
							onSubmit={async (i) => {
								await thread?.invoke(i);
							}}
							placeholder="What can I help with?"
						/>
					</div>
				{/if}
			</div>
		</div>

		<Confirm
			show={toDelete}
			msg={`Delete ${task.name}?`}
			onsuccess={deleteTask}
			oncancel={() => (toDelete = false)}
		/>

		<ResponsiveDialog bind:this={inputDialog} title="Run Task" class="max-w-full md:max-w-md">
			<div class="flex grow flex-col gap-4 p-4 md:p-0">
				<div class="mt-4 flex w-full md:mt-0">
					<Input bind:input {task} />
				</div>
				<div class="flex grow"></div>
				<div class="flex w-full flex-col justify-between gap-4 md:flex-row md:justify-end">
					<button
						class="button-primary w-full md:w-fit"
						onclick={() => {
							run();
							inputDialog?.close();
						}}>Run</button
					>
				</div>
			</div>
		</ResponsiveDialog>
	</div>
</div>

<style lang="postcss">
	.chat-overlay {
		opacity: 1;
		&::after {
			z-index: 20;
			content: '';
			position: absolute;
			top: -3rem;
			left: 0;
			width: 100%;
			height: 3rem;
			background: linear-gradient(to bottom, transparent, var(--background));
		}
	}
</style>

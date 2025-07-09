<script lang="ts">
	import {
		ChatService,
		getTaskRun,
		type Messages,
		type Project,
		type Task,
		type TaskStep
	} from '$lib/services';
	import { ChevronRight, MessageCircle, MessageCircleOff, Trash2, X } from 'lucide-svelte/icons';
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
	import Tools from '../navbar/Tools.svelte';
	import { clickOutside } from '$lib/actions/clickoutside';

	interface Props {
		task: Task;
		project: Project;
		onChanged?: (task: Task) => void | Promise<void>;
		onDelete?: () => void | Promise<void>;
		runID?: string;
	}

	let { task = $bindable(), onChanged, project, onDelete, runID: inputRunID }: Props = $props();

	const readOnly = !!inputRunID;
	let runID = $state(inputRunID);
	let thread: Thread | undefined = $state<Thread>();
	let lastStepId: string | undefined = $state(undefined);
	let stepMessages: Record<string, Messages> = $state({});
	let allMessages = $state<Messages>({ messages: [], inProgress: false });
	let input = $state('');
	let error = $state('');
	let pending = $derived(thread?.pending ?? false);
	// let running = $derived(allMessages.inProgress);
	let inputDialog = $state<HTMLDialogElement>();

	let showChat = $state(false);
	let showAllOutput = $state(true);

	let taskHeaderActionDiv: HTMLDivElement | undefined = $state<HTMLDivElement>();
	let isTaskInfoVisible = $state(true);
	let observer: IntersectionObserver;

	let taskRun: Task | undefined = $state(undefined);

	$effect(() => {
		// Load task run data only in readonly mode; so that we can get the correct steps & loop steps related to the current task
		if (readOnly && project && runID) {
			getTaskRun(project.assistantID, project.id, task.id, runID).then((res) => {
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

	/**************************************************************************************************/

	const saver = newSaveMonitor(
		() => task,
		async (t: Task) => {
			return await ChatService.saveTask(project.assistantID, project.id, t);
		},
		(t) => {
			task = t;
			onChanged?.(t);
		}
	);

	$effect(() => {
		if (task.id && task.steps.length === 0 && !readOnly) {
			task.steps.push({
				id: 'si1' + Math.random().toString(36).substring(6)
			});
		}
	});

	$effect(resetThread);

	let toDelete: boolean = $state(false);

	async function deleteTask() {
		if (readOnly) {
			return;
		}
		toDelete = false;
		await ChatService.deleteTask(project.assistantID, project.id, task.id);
		onDelete?.();
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
		task = await ChatService.getTask(project.assistantID, project.id, task.id);
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

	function isTaskFromIntegration(task: Task) {
		return (
			task.id === project.workflowNamesFromIntegration?.slackWorkflowName ||
			task.id === project.workflowNamesFromIntegration?.discordWorkflowName ||
			task.id === project.workflowNamesFromIntegration?.emailWorkflowName ||
			task.id === project.workflowNamesFromIntegration?.webhookWorkflowName
		);
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

		if (task.onDemand || task.email || task.webhook) {
			inputDialog?.showModal();
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
		{#if allMessages.messages.length > 0}
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
		{#if !readOnly && !isTaskFromIntegration(task)}
			<button
				class="bg-blue flex items-center justify-center gap-2 rounded-2xl px-12 py-2 text-white transition-all duration-200 hover:bg-blue-400"
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
			<h4 class="border-blue grow truncate border-l-4 pl-2 text-lg font-semibold md:text-xl">
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
					<div class="border-blue flex grow flex-col gap-1 border-l-4 pl-4">
						<strong class="text-blue text-xs">TASK</strong>

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
							class="flex h-full flex-col items-end justify-center gap-4 md:justify-between"
						>
							{#if !readOnly && !isTaskFromIntegration(task)}
								<button class="button-destructive p-4" onclick={() => (toDelete = true)}>
									<Trash2 class="size-4" />
								</button>
							{/if}
							{@render mainActions()}
						</div>
					{/if}
				</div>
				{#if responsive.isMobile}
					<div bind:this={taskHeaderActionDiv} class="flex w-full justify-between px-4">
						{#if !readOnly && !isTaskFromIntegration(task)}
							<button class="button-destructive p-4" onclick={() => (toDelete = true)}>
								<Trash2 class="size-4" />
							</button>
						{:else}
							<!-- placeholder -->
							<div class="size-4"></div>
						{/if}
						<div class="flex">
							{@render mainActions()}
						</div>
					</div>
				{/if}
			</div>
			<div class="flex w-full justify-center">
				<div
					class="flex w-full flex-col gap-4 rounded-xl bg-gray-50 p-4 shadow-inner md:max-w-[1200px] dark:bg-black"
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
				class="sticky bottom-0 flex items-center justify-center bg-white px-6 opacity-0 transition-opacity dark:bg-black"
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
						>
							<div class="flex w-fit items-center gap-1">
								<div use:tooltip={'Tools'}>
									<Tools {project} />
								</div>
							</div>
						</ChatInput>
					</div>
				{/if}
			</div>
		</div>

		<Confirm
			show={toDelete}
			msg="Are you sure you want to delete this task?"
			onsuccess={deleteTask}
			oncancel={() => (toDelete = false)}
		/>

		<dialog
			bind:this={inputDialog}
			use:clickOutside={() => inputDialog?.close()}
			class="max-w-full md:min-w-md"
			class:p-4={!responsive.isMobile}
			class:mobile-screen-dialog={responsive.isMobile}
		>
			<div class="flex h-full w-full flex-col justify-between gap-4">
				<h3 class="default-dialog-title" class:default-dialog-mobile-title={responsive.isMobile}>
					Run Task
					<button
						class:mobile-header-button={responsive.isMobile}
						onclick={() => inputDialog?.close()}
						class="icon-button"
					>
						{#if responsive.isMobile}
							<ChevronRight class="size-6" />
						{:else}
							<X class="size-5" />
						{/if}
					</button>
				</h3>
				<div class="flex w-full grow">
					<Input bind:input {task} />
				</div>
				<div class="mt-4 flex w-full flex-col justify-between gap-4 md:flex-row md:justify-end">
					<button
						class="button-primary w-full md:w-fit"
						onclick={() => {
							run();
							inputDialog?.close();
						}}>Run</button
					>
				</div>
			</div>
		</dialog>
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

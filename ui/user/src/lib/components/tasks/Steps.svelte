<script lang="ts">
	import { type Messages, type Project, type Task, type TaskStep } from '$lib/services';
	import Step from '$lib/components/tasks/Step.svelte';
	import { SvelteMap } from 'svelte/reactivity';
	import Files from '$lib/components/tasks/Files.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { Eye, EyeClosed, UsersRound, ArrowBigDown } from 'lucide-svelte';
	import { tick, untrack } from 'svelte';
	import { twMerge } from 'tailwind-merge';
	import { fade } from 'svelte/transition';
	import { linear } from 'svelte/easing';

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
		shouldFollowTaskRun?: boolean;
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
		readOnly,
		shouldFollowTaskRun = $bindable()
	}: Props = $props();

	const steps = $derived(task?.steps ?? []);

	// Capture the steps element
	let element: HTMLElement | undefined = $state();

	// Capture the parent scrollable element
	let scrollableElement: HTMLElement | undefined = $state();

	$effect(() => {
		// Make sure steps element is defined
		if (!element) return;

		// Find the closest scrollable parent element
		scrollableElement = element.closest('div[data-scrollable="true"]') as HTMLElement;
	});

	$effect(() => {
		// Run only during task run
		if (!running) return;

		// If scrollable element not found, break
		if (!scrollableElement) return;

		// If user is not following task run, break
		if (!shouldFollowTaskRun) return;

		// Scroll to the bottom each time the steps element height changed
		const onresize = () => {
			requestAnimationFrame(() => {
				scrollDown();
			});
		};

		const observer = new ResizeObserver(onresize);

		onresize();

		observer.observe(element!);

		return () => {
			observer.disconnect();
		};
	});

	$effect(() => {
		// If scrollable element is not yet ready, break
		if (!scrollableElement) return;

		// Task is not running; then no need to listen for scrolls
		if (!running) return;

		// capture the old scroll top value
		let previousScrollTop = scrollableElement.scrollTop;

		const onscroll = (ev: Event) => {
			const element = ev.currentTarget as HTMLElement;

			requestAnimationFrame(async () => {
				// Do not continue if the scrollable element hasn't reached its max-height yet
				if (element.clientHeight >= element.scrollHeight) return;

				// Await for pending calculation
				await tick();

				// Check if the user scrolled up
				if (previousScrollTop - element.scrollTop > 12) {
					// Make sure not to make this effect run again because of this assignment
					untrack(() => {
						// Exit following task run
						shouldFollowTaskRun = false;
					});
				}

				// Save the current scroll top value
				previousScrollTop = element.scrollTop;
			});
		};

		// Wait 1s for messages to collapse on re-run
		setTimeout(() => {
			scrollableElement!.addEventListener('scroll', onscroll);
		}, 1000);

		// Cleanup
		return () => {
			scrollableElement!.removeEventListener('scroll', onscroll);
		};
	});

	let hasScrollingContent = $state(false);
	let scrollDirection: 'up' | 'down' = $state('down');

	$effect(() => {
		if (!scrollableElement) return;

		const onscroll = () => {
			// Do not continue if the scrollable element hasn't reached its max-height yet
			requestAnimationFrame(async () => {
				setTimeout(() => {
					untrack(() => {
						if (!scrollableElement) return;
						// Exit following task run
						// Use setTimeout fn to make this less prioritized

						if (
							Math.ceil(scrollableElement.scrollHeight) > Math.ceil(scrollableElement.clientHeight)
						) {
							hasScrollingContent = true;
						}

						const maxScrollTop = Math.ceil(
							scrollableElement.scrollHeight - scrollableElement.clientHeight
						);

						const hasReachedBottom =
							Math.min(Math.ceil(scrollableElement.scrollTop), maxScrollTop) >= maxScrollTop;

						scrollDirection = hasReachedBottom ? 'up' : 'down';
					});
				}, 300);
			});
		};

		// Allow UI to breath before checking if the scroll is at the bottom
		setTimeout(() => {
			onscroll();
		}, 300);

		// Wait 1s for messages to collapse on re-run
		scrollableElement!.addEventListener('scroll', onscroll);

		// Cleanup
		return () => {
			scrollableElement!.removeEventListener('scroll', onscroll);
		};
	});

	function onNavigationClick() {
		if (!readOnly && running) {
			shouldFollowTaskRun = true;
		}

		requestAnimationFrame(() => {
			if (scrollDirection === 'down') {
				scrollDown();
			} else {
				scrollUp();
			}
		});
	}

	function scrollDown() {
		if (!scrollableElement) return;

		// Calculate scroll top
		const top = Math.max(
			scrollableElement!.clientHeight,
			scrollableElement!.scrollHeight - scrollableElement!.clientHeight
		);

		scrollableElement!.scrollTo({
			top,
			behavior: running && shouldFollowTaskRun ? 'instant' : 'smooth'
		});
	}

	function scrollUp() {
		if (!scrollableElement) return;
		scrollableElement!.scrollTo({
			top: 0,
			behavior: running && shouldFollowTaskRun ? 'instant' : 'smooth'
		});
	}
</script>

<div
	bind:this={element}
	class="task-steps dark:bg-surface1 dark:border-surface3 relative rounded-lg bg-white p-5 pb-10 shadow-sm dark:border"
>
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

	<ol class="flex list-decimal flex-col gap-2 pt-2 opacity-100">
		{#each steps as step, i (step.id)}
			<Step
				{run}
				{runID}
				bind:task
				bind:step={task.steps[i]}
				index={i}
				{stepMessages}
				{pending}
				{project}
				showOutput={showAllOutput}
				{readOnly}
			/>
		{/each}
	</ol>

	{#if error}
		<div class="mt-2 text-red-500">{error}</div>
	{/if}

	{#if (!readOnly && running) || hasScrollingContent}
		{@const isFollowModeActive = !readOnly && running && shouldFollowTaskRun}

		<div class="pointer-events-none absolute inset-0 z-10 flex items-end justify-end p-4">
			<button
				class={twMerge(
					'bg-surface2 pointer-events-auto sticky right-0 bottom-4 box-border flex aspect-square h-8 items-center justify-center rounded-lg transition-colors duration-200',
					isFollowModeActive &&
						'bg-blue/0 text-blue/70 hover:bg-blue/10 active:bg-blue/20 border border-current'
				)}
				onclick={onNavigationClick}
				in:fade={{ duration: 100, delay: 0, easing: linear }}
				out:fade={{ duration: 50, delay: 0, easing: linear }}
			>
				<div
					class="h-4 w-4 duration-200"
					class:rotate={!isFollowModeActive && scrollDirection === 'up'}
				>
					{#if isFollowModeActive}
						<UsersRound class="h-full w-full" />
					{:else}
						<ArrowBigDown class="h-full w-full" />
					{/if}
				</div>
			</button>
		</div>
	{/if}
</div>

{#if runID}
	<Files taskID={task.id} {runID} running={running || pending} {project} />
{/if}

<style>
	.rotate {
		transition-property: transform;
		transition-duration: var(--tw-duration);
		transform: rotate(180deg);
	}
</style>

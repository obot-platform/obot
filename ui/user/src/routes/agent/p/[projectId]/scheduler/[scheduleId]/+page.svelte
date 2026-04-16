<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import ScheduledTaskDialog from '$lib/components/nanobot/ScheduledTaskDialog.svelte';
	import {
		formatScheduleDate,
		formatScheduleDateTime,
		scheduleSummary
	} from '$lib/components/nanobot/taskSchedule';
	import type {
		Chat,
		ProjectLayoutContext,
		ResourceContents,
		ScheduledTask
	} from '$lib/services/nanobot/types';
	import { PROJECT_LAYOUT_CONTEXT } from '$lib/services/nanobot/types';
	import { errors, timePreference } from '$lib/stores';
	import { nanobotChat } from '$lib/stores/nanobotChat.svelte';
	import { goto } from '$lib/url';
	import ConfirmScheduleToggle from '../ConfirmScheduleToggle.svelte';
	import { CalendarClock, PencilLine, Play, Timer, TimerOff, Trash2 } from 'lucide-svelte';
	import { getContext, tick } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	let { data } = $props();
	let projectId = $derived(data.projectId ?? data.projects[0].id);
	let taskId = $derived(data.scheduleId);
	let taskURI = $derived(`task:///${taskId}`);

	let task = $state<ScheduledTask>();
	let sessions = $state<Chat[]>([]);
	let loadingTask = $state(false);
	let loadError = $state('');
	let deleting = $state(false);
	let runningNow = $state(false);
	let confirmDelete = $state(false);
	let confirmToggleEnabled = $state<
		| {
				uri: string;
				name: string;
				enabled?: boolean;
				schedule?: string;
				expiration?: string;
		  }
		| undefined
	>(undefined);
	let updatingEnabled = $state(false);
	let toggleHover = $state(false);
	let toggleMeasureRestEl = $state<HTMLDivElement | undefined>(undefined);
	let toggleMeasureHoverEl = $state<HTMLDivElement | undefined>(undefined);
	let toggleWidthRest = $state(0);
	let toggleWidthHover = $state(0);
	let editDialog = $state<ReturnType<typeof ScheduledTaskDialog>>();
	let taskContainer = $state<HTMLElement | undefined>(undefined);
	let loadedTaskURI = $state('');
	const projectLayout = getContext<ProjectLayoutContext>(PROJECT_LAYOUT_CONTEXT);

	const sortedSessions = $derived.by(() =>
		[...sessions].sort((a, b) => new Date(b.created).getTime() - new Date(a.created).getTime())
	);

	function updateToggleLabelWidths() {
		const rest = toggleMeasureRestEl;
		const hover = toggleMeasureHoverEl;
		if (!rest || !hover) return;
		// Ceil + small buffer: subpixel/layout can otherwise clip the last glyphs vs off-screen measure.
		const pad = 4;
		toggleWidthRest = Math.ceil(rest.getBoundingClientRect().width) + pad;
		toggleWidthHover = Math.ceil(hover.getBoundingClientRect().width) + pad;
	}

	$effect(() => {
		if (!task) return;
		void task.enabled;
		tick().then(updateToggleLabelWidths);
	});

	$effect(() => {
		if (!toggleMeasureRestEl || !toggleMeasureHoverEl || !task) return;
		updateToggleLabelWidths();
	});

	$effect(() => {
		const container = taskContainer;
		if (!container) return;

		const ro = new ResizeObserver((entries) => {
			const entry = entries[0];
			projectLayout.setThreadContentWidth(entry.contentRect.width);
		});
		ro.observe(container);
		projectLayout.setThreadContentWidth(container.getBoundingClientRect().width);
		return () => ro.disconnect();
	});

	$effect(() => {
		if (!$nanobotChat?.api || loadedTaskURI === taskURI) return;
		loadedTaskURI = taskURI;
		loadTask();
		loadSessions();
	});

	$effect(() => {
		if (task) {
			projectLayout.setLayoutName(task.name);
			projectLayout.setShowBackButton(true);
		}
	});

	function parseTask(content?: ResourceContents): ScheduledTask {
		if (!content?.text) {
			throw new Error('Scheduled task contents were empty');
		}
		const parsed = JSON.parse(content.text) as ScheduledTask;
		return {
			...parsed,
			uri: parsed.uri || content.uri || taskURI
		};
	}

	async function refreshResources() {
		if (!$nanobotChat?.api) return;
		try {
			const resources = await $nanobotChat.api.listResources();
			nanobotChat.update((state) => {
				if (state) {
					state.resources = resources;
				}
				return state;
			});
		} catch (error) {
			errors.append(error);
		}
	}

	async function loadTask() {
		if (!$nanobotChat?.api) return;
		loadingTask = true;
		loadError = '';
		try {
			const result = await $nanobotChat.api.readResource(taskURI);
			task = parseTask(result.contents?.[0]);
		} catch (error) {
			loadError = error instanceof Error ? error.message : 'Failed to load schedule';
			errors.append(error);
		} finally {
			loadingTask = false;
		}
	}

	async function loadSessions() {
		if (!$nanobotChat?.api) return;
		try {
			const allSessions = await $nanobotChat.api.listSessions();
			sessions = allSessions.filter((session) => session.taskURI === taskURI);
		} catch (error) {
			errors.append(error);
		}
	}

	function openSession(sessionId: string) {
		goto(`/agent/p/${projectId}?tid=${sessionId}&sid=${taskId}`);
	}

	function handleSessionRowKeydown(event: KeyboardEvent, sessionId: string) {
		if (event.key === 'Enter' || event.key === ' ') {
			event.preventDefault();
			openSession(sessionId);
		}
	}

	async function handleTaskSaved(savedTask: ScheduledTask) {
		task = savedTask;
		await refreshResources();
		await loadTask();
	}

	async function handleRunNow() {
		if (!$nanobotChat?.api || !task) return;
		runningNow = true;
		try {
			const response = await $nanobotChat.api.startScheduledTask(task.uri);
			goto(`/agent/p/${projectId}?tid=${response.sessionId}`);
		} catch (error) {
			errors.append(error);
		} finally {
			runningNow = false;
		}
	}

	async function handleToggleEnabled() {
		if (!$nanobotChat?.api || !task || updatingEnabled) return;
		updatingEnabled = true;
		try {
			await $nanobotChat.api.updateScheduledTask({
				uri: task.uri,
				name: task.name,
				prompt: task.prompt,
				schedule: task.schedule,
				timezone: task.timezone,
				expiration: task.expiration,
				enabled: !task.enabled
			});
			await refreshResources();
			await loadTask();
		} catch (error) {
			errors.append(error);
		} finally {
			updatingEnabled = false;
			confirmToggleEnabled = undefined;
		}
	}

	async function handleDeleteTask() {
		if (!$nanobotChat?.api || !task) return;
		deleting = true;
		try {
			await $nanobotChat.api.deleteScheduledTask(task.uri);
			await refreshResources();
			goto(`/agent/p/${projectId}/scheduler`);
		} catch (error) {
			errors.append(error);
		} finally {
			deleting = false;
			confirmDelete = false;
		}
	}
</script>

<svelte:head>
	<title>Obot | {task?.name || 'Schedule'}</title>
</svelte:head>

<div class="mx-auto flex w-full max-w-4xl flex-col gap-6 px-4 md:px-8" bind:this={taskContainer}>
	{#if loadingTask && !task}
		<div class="flex flex-col gap-4">
			<div class="flex justify-between items-center gap-4">
				<div class="skeleton h-10 w-21"></div>
				<div class="skeleton h-10 w-10"></div>
			</div>

			<div class="skeleton h-39 w-full"></div>
		</div>
	{:else if loadError && !task}
		<div class="alert alert-error">
			<span>{loadError}</span>
		</div>
	{:else if task}
		<div
			class="pointer-events-none fixed top-0 left-[-10000px] z-[-1] flex flex-col gap-0"
			aria-hidden="true"
		>
			<div bind:this={toggleMeasureRestEl} class="flex w-max items-center gap-2 text-xs">
				{#if task.enabled}
					<Timer class="size-4 shrink-0" />
					<span class="font-light">Active</span>
				{:else}
					<TimerOff class="size-4 shrink-0" />
					<span class="font-light">Inactive</span>
				{/if}
			</div>
			<div bind:this={toggleMeasureHoverEl} class="flex w-max items-center gap-2 text-xs">
				{#if task.enabled}
					<TimerOff class="size-4 shrink-0" />
					<span class="font-medium">Disable this schedule?</span>
				{:else}
					<Timer class="size-4 shrink-0" />
					<span class="font-medium">Enable this schedule?</span>
				{/if}
			</div>
		</div>
		<div class="flex flex-wrap items-start justify-between gap-4">
			<div class="flex items-center gap-3">
				<button class="btn" onclick={() => editDialog?.open(task)}>
					<PencilLine class="size-4 shrink-0" /> Edit
				</button>
				<button
					type="button"
					class={twMerge(
						'btn',
						task.enabled ? 'btn-success btn-soft hover:btn-neutral' : 'hover:btn-success'
					)}
					disabled={updatingEnabled}
					aria-label={task.enabled ? 'Disable this schedule' : 'Enable this schedule'}
					onmouseenter={() => (toggleHover = true)}
					onmouseleave={() => (toggleHover = false)}
					onclick={() =>
						(confirmToggleEnabled = {
							uri: taskURI,
							name: task?.name ?? taskId,
							enabled: task?.enabled,
							schedule: task?.schedule,
							expiration: task?.expiration
						})}
				>
					<div
						class="flex shrink-0 items-center gap-2 overflow-hidden text-xs whitespace-nowrap transition-[width] duration-200 ease-out motion-reduce:transition-none"
						style:width={toggleWidthRest > 0 && toggleWidthHover > 0
							? `${toggleHover ? toggleWidthHover : toggleWidthRest}px`
							: undefined}
					>
						{#if toggleHover}
							{#if task.enabled}
								<TimerOff class="size-4 shrink-0" />
								<span class="font-medium">Disable this schedule?</span>
							{:else}
								<Timer class="size-4 shrink-0" />
								<span class="font-medium">Enable this schedule?</span>
							{/if}
						{:else if task.enabled}
							<Timer class="size-4 shrink-0" />
							<span class="font-light">Active</span>
						{:else}
							<TimerOff class="size-4 shrink-0" />
							<span class="font-light">Inactive</span>
						{/if}
					</div>
				</button>
			</div>
			<div class="flex flex-wrap items-center gap-2">
				<button
					class="btn btn-error btn-soft btn-square tooltip tooltip-left"
					data-tip="Delete schedule"
					aria-label="Delete schedule"
					onclick={() => (confirmDelete = true)}
				>
					<Trash2 class="size-4" />
				</button>
			</div>
		</div>

		<div class="bg-base-100 rounded-box border-base-300 border">
			<div class="grid gap-0 md:grid-cols-2">
				<div class="border-base-300 border-b px-5 py-4 md:border-r">
					<div class="text-base-content/50 text-xs font-medium uppercase">Schedule</div>
					<div class="mt-2 text-sm">
						{scheduleSummary(task.schedule, task.expiration, timePreference.timeFormat)}
					</div>
				</div>
				<div class="border-base-300 border-b px-5 py-4">
					<div class="text-base-content/50 text-xs font-medium uppercase">Next run</div>
					<div class="mt-2 text-sm">
						{formatScheduleDateTime(task.nextRunAt, timePreference.timeFormat)}
					</div>
				</div>
				<div class="border-base-300 px-5 py-4 md:border-r">
					<div class="text-base-content/50 text-xs font-medium uppercase">Expiration</div>
					<div class="mt-2 text-sm">
						{task.expiration ? formatScheduleDate(task.expiration) : 'No expiration'}
					</div>
				</div>
				<div class="px-5 py-4">
					<div class="text-base-content/50 text-xs font-medium uppercase">Last run</div>
					<div class="mt-2 text-sm">
						{formatScheduleDateTime(sortedSessions[0]?.created, timePreference.timeFormat)}
					</div>
				</div>
			</div>
		</div>

		<div class="bg-base-100 rounded-box border-base-300 border p-5">
			<div class="mb-3">
				<h3 class="text-lg font-semibold">Prompt</h3>
			</div>
			<div
				class="bg-base-200/40 border-base-300 rounded-xl border px-4 py-4 text-sm leading-6 whitespace-pre-wrap"
			>
				{task.prompt}
			</div>
		</div>

		<div class="mb-8 flex flex-col gap-4">
			<div class="divider"></div>
			<div class="flex items-center justify-between">
				<h2 class="text-xl font-semibold">Runs</h2>
				<button class="btn btn-sm btn-primary" onclick={handleRunNow} disabled={runningNow}>
					Run Now <Play class="size-4" />
				</button>
			</div>

			{#if sortedSessions.length === 0}
				<div
					class="bg-base-200/60 rounded-box flex flex-col items-center gap-3 px-6 py-10 text-center"
				>
					<div class="bg-base-100 rounded-full p-4">
						<CalendarClock class="size-7" />
					</div>
					<div class="space-y-1">
						<h3 class="font-medium">No runs yet</h3>
						<p class="text-base-content/60 text-sm">
							This schedule has not started any sessions yet.
						</p>
					</div>
				</div>
			{:else}
				<div class="overflow-hidden">
					<table class="table">
						<thead>
							<tr>
								<th>Title</th>
								<th>Started</th>
							</tr>
						</thead>
						<tbody>
							{#each sortedSessions as session (session.id)}
								<tr
									role="button"
									tabindex="0"
									class="hover:bg-base-200 cursor-pointer"
									onclick={() => openSession(session.id)}
									onkeydown={(event) => handleSessionRowKeydown(event, session.id)}
								>
									<td class="min-w-0">
										<div class="truncate font-medium">{session.title || 'Untitled Session'}</div>
									</td>
									<td class="text-base-content/60 text-sm">
										{formatScheduleDateTime(session.created, timePreference.timeFormat)}
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}
		</div>
	{/if}
</div>

{#if $nanobotChat?.api}
	<ScheduledTaskDialog bind:this={editDialog} api={$nanobotChat.api} onSaved={handleTaskSaved} />
{/if}

<Confirm
	show={confirmDelete}
	title="Delete Schedule"
	msg={`Delete ${task?.name ?? taskId}?`}
	note="Existing run sessions will remain, but this schedule will stop creating new ones."
	loading={deleting}
	onsuccess={handleDeleteTask}
	oncancel={() => (confirmDelete = false)}
/>

<ConfirmScheduleToggle
	task={confirmToggleEnabled}
	loading={updatingEnabled}
	onSuccess={handleToggleEnabled}
	onCancel={() => (confirmToggleEnabled = undefined)}
/>

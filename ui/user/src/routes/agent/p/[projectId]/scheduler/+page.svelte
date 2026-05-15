<script lang="ts">
	import { afterNavigate } from '$app/navigation';
	import Confirm from '$lib/components/Confirm.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import ScheduledTaskDialog from '$lib/components/nanobot/ScheduledTaskDialog.svelte';
	import { scheduleSummary } from '$lib/components/nanobot/taskSchedule';
	import type {
		ProjectLayoutContext,
		Resource,
		ResourceContents,
		ScheduledTask
	} from '$lib/services/nanobot/types';
	import { PROJECT_LAYOUT_CONTEXT } from '$lib/services/nanobot/types';
	import { errors, timePreference } from '$lib/stores';
	import { nanobotChat } from '$lib/stores/nanobotChat.svelte';
	import { goto } from '$lib/url';
	import ConfirmScheduleToggle from './ConfirmScheduleToggle.svelte';
	import { EllipsisVertical, Play, Plus, Search, Timer, TimerOff, Trash2 } from 'lucide-svelte';
	import { getContext } from 'svelte';

	let { data } = $props();
	let projectId = $derived(data.projectId ?? data.projects[0].id);

	let createDialog = $state<ReturnType<typeof ScheduledTaskDialog>>();
	let deleting = $state(false);
	let loading = $state(false);
	let mutatingTaskURI = $state('');
	let taskQuery = $state('');
	let confirmDeleteTask = $state<
		| {
				uri: string;
				name: string;
		  }
		| undefined
	>();
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
	let tasksContainer = $state<HTMLElement | undefined>(undefined);
	let refreshingTaskData = $state<Promise<void> | undefined>(undefined);
	const projectLayout = getContext<ProjectLayoutContext>(PROJECT_LAYOUT_CONTEXT);

	type TaskResource = Resource & {
		_meta?: {
			['ai.nanobot.meta/task']?: {
				createdAt?: string;
				schedule?: string;
				enabled?: boolean;
				expiration?: string;
				timezone?: string;
			};
			[key: string]: unknown;
		};
	};

	function taskMeta(task: TaskResource) {
		return task._meta?.['ai.nanobot.meta/task'];
	}

	function taskIDFor(resource: { uri: string }) {
		return resource.uri.replace('task:///', '');
	}

	function parseTask(content: ResourceContents, fallbackURI: string): ScheduledTask {
		if (!content.text) {
			throw new Error('Scheduled task contents were empty');
		}

		const parsed = JSON.parse(content.text) as ScheduledTask;
		return {
			...parsed,
			uri: parsed.uri || content.uri || fallbackURI
		};
	}

	let tasks = $derived.by(() =>
		(($nanobotChat?.resources ?? []) as TaskResource[])
			.filter((resource) => resource.uri.startsWith('task:///'))
			.sort((a, b) => (a.name ?? '').localeCompare(b.name ?? ''))
	);

	let filteredTasks = $derived.by(() => {
		const query = taskQuery.trim().toLowerCase();
		if (!query) {
			return tasks;
		}

		return tasks.filter((task) => {
			const summary = scheduleSummary(
				String(taskMeta(task)?.schedule ?? ''),
				taskMeta(task)?.expiration,
				timePreference.timeFormat
			).toLowerCase();
			return (task.name ?? '').toLowerCase().includes(query) || summary.includes(query);
		});
	});

	$effect(() => {
		const container = tasksContainer;
		if (!container) return;

		const ro = new ResizeObserver((entries) => {
			const entry = entries[0];
			projectLayout.setThreadContentWidth(entry.contentRect.width);
		});
		ro.observe(container);
		projectLayout.setThreadContentWidth(container.getBoundingClientRect().width);
		return () => ro.disconnect();
	});

	afterNavigate(({ from }) => {
		if (!from?.url || !$nanobotChat?.api) return;
		void refreshTaskData();
	});

	async function refreshResources() {
		if (!$nanobotChat?.api) return;
		loading = true;
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
		} finally {
			loading = false;
		}
	}

	async function refreshSessions() {
		if (!$nanobotChat?.api) return;
		try {
			const sessions = await $nanobotChat.api.listSessions();
			nanobotChat.update((state) => {
				if (state) {
					state.sessions = sessions;
				}
				return state;
			});
		} catch (error) {
			errors.append(error);
		}
	}

	async function refreshTaskData() {
		if (refreshingTaskData) {
			return refreshingTaskData;
		}

		refreshingTaskData = Promise.all([refreshResources(), refreshSessions()])
			.then(() => undefined)
			.finally(() => {
				refreshingTaskData = undefined;
			});

		return refreshingTaskData;
	}

	$effect(() => {
		const api = $nanobotChat?.api;
		if (!api) return;

		const handleChange = () => {
			void refreshTaskData();
		};

		const stopListChanged = api.watchListChanged(handleChange);

		return () => {
			stopListChanged();
		};
	});

	async function handleTaskCreated(task: ScheduledTask) {
		await refreshTaskData();
		goto(`/agent/p/${projectId}/scheduler/${encodeURIComponent(taskIDFor(task))}`);
	}

	async function handleDeleteTask() {
		if (!confirmDeleteTask || !$nanobotChat?.api) return;
		deleting = true;
		try {
			await $nanobotChat.api.deleteScheduledTask(confirmDeleteTask.uri);
			await refreshTaskData();
			confirmDeleteTask = undefined;
		} catch (error) {
			errors.append(error);
		} finally {
			deleting = false;
		}
	}

	async function handleRunTask(uri: string) {
		if (!$nanobotChat?.api) return;
		mutatingTaskURI = uri;
		try {
			const response = await $nanobotChat.api.startScheduledTask(uri);
			goto(`/agent/p/${projectId}?tid=${response.sessionId}`);
		} catch (error) {
			errors.append(error);
		} finally {
			mutatingTaskURI = '';
		}
	}

	async function handleToggleTask(task: TaskResource) {
		if (!$nanobotChat?.api) return;
		mutatingTaskURI = task.uri;
		try {
			const read = await $nanobotChat.api.readResource(task.uri);
			const content = read.contents?.[0];
			if (!content) {
				throw new Error('Scheduled task contents were empty');
			}
			const currentTask = parseTask(content, task.uri);

			await $nanobotChat.api.updateScheduledTask({
				uri: currentTask.uri,
				name: currentTask.name,
				prompt: currentTask.prompt,
				schedule: currentTask.schedule,
				timezone: currentTask.timezone,
				expiration: currentTask.expiration,
				enabled: !currentTask.enabled
			});
			await refreshTaskData();
		} catch (error) {
			errors.append(error);
		} finally {
			mutatingTaskURI = '';
			confirmToggleEnabled = undefined;
		}
	}

	function openTask(task: { uri: string }) {
		goto(`/agent/p/${projectId}/scheduler/${encodeURIComponent(taskIDFor(task))}`);
	}

	function handleRowKeydown(event: KeyboardEvent, task: { uri: string }) {
		if (event.key === 'Enter' || event.key === ' ') {
			event.preventDefault();
			openTask(task);
		}
	}
</script>

<svelte:head>
	<title>Obot | Scheduler</title>
</svelte:head>

<div class="mx-auto flex w-full max-w-4xl flex-col gap-6 px-4 md:px-8" bind:this={tasksContainer}>
	<div class="flex items-center justify-between gap-3">
		<div class="flex items-center gap-1">
			<h2 class="text-xl font-semibold md:text-2xl">Scheduler</h2>
			{#if loading}
				<div class="loading loading-spinner loading-sm text-primary ml-2"></div>
			{/if}
		</div>

		<button
			class="btn btn-primary btn-circle"
			aria-label="Create schedule"
			onclick={() => createDialog?.open()}
		>
			<Plus class="size-5 text-primary-content" />
		</button>
	</div>

	<label class="input mt-1 w-full">
		<Search class="size-5" />
		<input type="search" placeholder="Search schedules..." bind:value={taskQuery} />
	</label>

	<table class="mb-8 table">
		<thead>
			<tr>
				<th>Title</th>
				<th>Schedule</th>
				<th>Status</th>
				<th class="w-0"></th>
			</tr>
		</thead>
		<tbody>
			{#if filteredTasks.length > 0}
				{#each filteredTasks as task (task.uri)}
					<tr
						class="hover:bg-base-200 cursor-pointer"
						role="button"
						tabindex="0"
						onclick={() => openTask(task)}
						onkeydown={(event) => handleRowKeydown(event, task)}
					>
						<td class="font-medium">{task.name}</td>
						<td class="text-base-content/70">
							{scheduleSummary(
								String(taskMeta(task)?.schedule ?? ''),
								taskMeta(task)?.expiration,
								timePreference.timeFormat
							)}
						</td>
						<td>
							<span
								class={`badge badge-sm ${taskMeta(task)?.enabled ? 'badge-success badge-soft' : 'badge-neutral badge-soft'}`}
							>
								{taskMeta(task)?.enabled ? 'Enabled' : 'Disabled'}
							</span>
						</td>
						<td class="text-right" onclick={(event) => event.stopPropagation()}>
							<DotDotDot
								class="hover:bg-base-200 dark:hover:bg-base-400"
								placement="bottom-end"
								disablePortal
								classes={{
									menu: 'min-w-48 rounded-2xl border border-base-300 p-1.5 shadow-xl'
								}}
							>
								{#snippet icon()}
									<EllipsisVertical class="size-4" />
								{/snippet}
								{#snippet children({ toggle })}
									<button
										type="button"
										class="hover:bg-base-200 dark:hover:bg-base-400 flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-left text-sm transition-colors"
										disabled={mutatingTaskURI === task.uri}
										onclick={async (event) => {
											event.preventDefault();
											event.stopPropagation();
											toggle(false);
											await handleRunTask(task.uri);
										}}
									>
										<Play class="size-4 shrink-0" />
										Run Now
									</button>
									<button
										type="button"
										class="hover:bg-base-200 dark:hover:bg-base-400 flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-left text-sm transition-colors"
										disabled={mutatingTaskURI === task.uri}
										onclick={async (event) => {
											event.preventDefault();
											event.stopPropagation();
											toggle(false);

											const meta = taskMeta(task);
											confirmToggleEnabled = {
												...meta,
												name: task.name,
												uri: task.uri
											};
										}}
									>
										{#if taskMeta(task)?.enabled}
											<TimerOff class="size-4 shrink-0" />
										{:else}
											<Timer class="size-4 shrink-0" />
										{/if}
										{taskMeta(task)?.enabled ? 'Disable' : 'Enable'}
									</button>
									<button
										type="button"
										class="text-error hover:bg-error/10 flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-left text-sm transition-colors"
										onclick={(event) => {
											event.preventDefault();
											event.stopPropagation();
											toggle(false);
											confirmDeleteTask = {
												uri: task.uri,
												name: task.name
											};
										}}
									>
										<Trash2 class="size-4 shrink-0" />
										Delete
									</button>
								{/snippet}
							</DotDotDot>
						</td>
					</tr>
				{/each}
			{:else}
				<tr>
					<td colspan="4" class="text-muted-content text-center text-sm font-light italic">
						{taskQuery.trim() ? 'No schedules found.' : 'No schedules yet.'}
					</td>
				</tr>
			{/if}
		</tbody>
	</table>
</div>

{#if $nanobotChat?.api}
	<ScheduledTaskDialog
		bind:this={createDialog}
		api={$nanobotChat.api}
		onSaved={handleTaskCreated}
	/>
{/if}

<Confirm
	show={!!confirmDeleteTask}
	title="Delete Schedule"
	msg={`Delete ${confirmDeleteTask?.name ?? 'this schedule'}?`}
	note="Existing run sessions will remain, but this schedule will stop creating new ones."
	loading={deleting}
	onsuccess={handleDeleteTask}
	oncancel={() => (confirmDeleteTask = undefined)}
/>

<ConfirmScheduleToggle
	task={confirmToggleEnabled}
	loading={!!mutatingTaskURI}
	onSuccess={() => {
		if (!confirmToggleEnabled) return;
		handleToggleTask(confirmToggleEnabled);
	}}
	onCancel={() => (confirmToggleEnabled = undefined)}
/>

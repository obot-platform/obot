<script lang="ts">
	import DatePicker from '$lib/components/DatePicker.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Select from '$lib/components/Select.svelte';
	import TimeInput from '$lib/components/TimeInput.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import type { ChatAPI } from '$lib/services/nanobot/chat/index.svelte';
	import type {
		CreateScheduledTaskRequest,
		ScheduledTask,
		UpdateScheduledTaskRequest
	} from '$lib/services/nanobot/types';
	import { errors, timePreference } from '$lib/stores';
	import {
		buildCronSchedule,
		defaultTaskScheduleForm,
		formatMonthDaySummary,
		formatScheduleDate,
		joinNatural,
		ordinal,
		parseCronSchedule,
		type TaskFrequency
	} from './taskSchedule';
	import { Check, ChevronDown } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		api: ChatAPI;
		onSaved?: (task: ScheduledTask) => void | Promise<void>;
		onClose?: () => void;
	}

	type SelectOption = {
		id: string;
		label: string;
	};

	const repeatOptions: SelectOption[] = [
		{ id: 'daily', label: 'Daily' },
		{ id: 'weekly', label: 'Weekly' },
		{ id: 'monthly', label: 'Monthly' },
		{ id: 'no_repeat', label: 'No Repeat' }
	];

	const weekdayOptions = [
		{ value: 'mon', label: 'Monday', shortLabel: 'Mon' },
		{ value: 'tue', label: 'Tuesday', shortLabel: 'Tue' },
		{ value: 'wed', label: 'Wednesday', shortLabel: 'Wed' },
		{ value: 'thu', label: 'Thursday', shortLabel: 'Thu' },
		{ value: 'fri', label: 'Friday', shortLabel: 'Fri' },
		{ value: 'sat', label: 'Saturday', shortLabel: 'Sat' },
		{ value: 'sun', label: 'Sunday', shortLabel: 'Sun' }
	];

	const monthDays = Array.from({ length: 31 }, (_, index) => index + 1);
	let { api, onSaved, onClose }: Props = $props();
	const browserTimezone = Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let currentTask = $state<ScheduledTask>();
	let saving = $state(false);
	let errorMessage = $state('');
	let openSchedulePicker = $state<'weekly' | 'monthly' | null>(null);
	let weeklyPickerEl = $state<HTMLDivElement>();
	let monthlyPickerEl = $state<HTMLDivElement>();
	let name = $state('');
	let prompt = $state('');
	let frequency = $state<TaskFrequency>('daily');
	let time = $state('09:00');
	let timeAsDate = $derived.by(() => {
		const [h, m] = time.split(':').map(Number);
		return new Date(2000, 0, 1, h || 0, m || 0);
	});
	let date = $state('');
	let expiration = $state('');
	let timezone = $state(browserTimezone);
	let daysOfWeek = $state<string[]>([]);
	let daysOfMonth = $state<number[]>([]);
	let enabled = $state(true);

	function dateFromValue(value: string): Date | null {
		if (!value) return null;
		const [year, month, day] = value.split('-').map(Number);
		if (!year || !month || !day) return null;
		return new Date(year, month - 1, day);
	}

	function valueFromDate(value: Date | null): string {
		if (!value) return '';
		const year = value.getFullYear();
		const month = String(value.getMonth() + 1).padStart(2, '0');
		const day = String(value.getDate()).padStart(2, '0');
		return `${year}-${month}-${day}`;
	}

	function resetForm(task?: ScheduledTask) {
		currentTask = task;
		errorMessage = '';
		openSchedulePicker = null;
		name = task?.name ?? '';
		prompt = task?.prompt ?? '';
		enabled = task?.enabled ?? true;
		expiration = task?.expiration ?? '';
		timezone = task?.timezone || browserTimezone;

		const parsed = task
			? parseCronSchedule(task.schedule, task.expiration)
			: defaultTaskScheduleForm();
		frequency = parsed.frequency;
		time = parsed.time;
		date = parsed.date;
		daysOfWeek = [...parsed.daysOfWeek];
		daysOfMonth = [...parsed.daysOfMonth];
	}

	export function open(task?: ScheduledTask) {
		resetForm(task);
		dialog?.open();
	}

	function handleClose() {
		if (saving) return;
		errorMessage = '';
		onClose?.();
	}

	function toggleWeekday(day: string) {
		daysOfWeek = daysOfWeek.includes(day)
			? daysOfWeek.filter((value) => value !== day)
			: [...daysOfWeek, day];
	}

	function toggleMonthDay(day: number) {
		daysOfMonth = daysOfMonth.includes(day)
			? daysOfMonth.filter((value) => value !== day)
			: [...daysOfMonth, day];
	}

	function weekdayValueLabel() {
		return daysOfWeek.length > 0
			? joinNatural(
					weekdayOptions
						.filter((option) => daysOfWeek.includes(option.value))
						.map((option) => option.shortLabel)
				)
			: 'Select days';
	}

	function monthDayValueLabel() {
		return daysOfMonth.length > 0 ? formatMonthDaySummary(daysOfMonth) : 'Select days';
	}

	function sectionInputClass(error = false) {
		return twMerge(
			'text-input-filled border-base-300 min-h-12 rounded-xl border px-4 py-3 text-base shadow-none',
			error && 'error'
		);
	}

	function pickerPanelClass() {
		return 'bg-base-100 border-base-300 absolute top-[calc(100%+0.5rem)] right-0 left-0 z-50 max-h-80 overflow-y-auto rounded-xl border p-2 shadow-xl';
	}

	function schedulePickerButtonLabel(kind: 'weekly' | 'monthly') {
		return kind === 'weekly' ? weekdayValueLabel() : monthDayValueLabel();
	}

	function toggleSchedulePicker(kind: 'weekly' | 'monthly') {
		openSchedulePicker = openSchedulePicker === kind ? null : kind;
	}

	function handleWindowPointerDown(event: PointerEvent) {
		const target = event.target;
		if (!(target instanceof Node)) {
			openSchedulePicker = null;
			return;
		}
		if (weeklyPickerEl?.contains(target) || monthlyPickerEl?.contains(target)) {
			return;
		}
		openSchedulePicker = null;
	}

	async function handleSubmit() {
		errorMessage = '';
		if (!name.trim() || !prompt.trim()) {
			errorMessage = 'Title and prompt are required.';
			return;
		}
		if (!time) {
			errorMessage = 'A time is required.';
			return;
		}
		if (frequency === 'weekly' && daysOfWeek.length === 0) {
			errorMessage = 'Pick at least one weekday.';
			return;
		}
		if (frequency === 'monthly' && daysOfMonth.length === 0) {
			errorMessage = 'Pick at least one day of the month.';
			return;
		}
		if (frequency === 'no_repeat' && !date) {
			errorMessage = 'Pick a date for a one-time schedule.';
			return;
		}

		const schedule = buildCronSchedule({
			frequency,
			time,
			date,
			daysOfWeek,
			daysOfMonth
		});
		const resolvedExpiration = frequency === 'no_repeat' ? expiration || date : expiration;

		saving = true;
		try {
			const payload = {
				name: name.trim(),
				prompt: prompt.trim(),
				schedule,
				timezone,
				expiration: resolvedExpiration || '',
				enabled
			};

			const savedTask = currentTask
				? await api.updateScheduledTask({
						uri: currentTask.uri,
						...payload
					} satisfies UpdateScheduledTaskRequest)
				: await api.createScheduledTask(payload satisfies CreateScheduledTaskRequest);

			await onSaved?.(savedTask);
			dialog?.close();
		} catch (error) {
			errorMessage = error instanceof Error ? error.message : 'Failed to save schedule';
			errors.append(error);
		} finally {
			saving = false;
		}
	}
</script>

<svelte:window onpointerdown={handleWindowPointerDown} />

<ResponsiveDialog
	bind:this={dialog}
	onClose={handleClose}
	title={currentTask ? 'Edit Schedule' : 'Add Schedule'}
	class="w-full max-w-3xl"
	classes={{
		title: 'text-2xl font-semibold',
		content: 'max-h-[90dvh] overflow-y-auto px-6 pb-6 md:px-8 md:pb-8'
	}}
>
	<div class="flex flex-col gap-6">
		<div class="flex flex-col gap-3">
			<label for="schedule-title" class="input-label text-base font-medium">Title</label>
			<input
				id="schedule-title"
				class={sectionInputClass()}
				bind:value={name}
				placeholder="Summary of AI news"
			/>
		</div>

		<div class="flex flex-col gap-3">
			<label for="schedule-prompt" class="input-label text-base font-medium">Prompt</label>
			<textarea
				id="schedule-prompt"
				class="text-input-filled border-base-300 min-h-52 resize-y rounded-xl border px-4 py-4 text-base shadow-none"
				bind:value={prompt}
				placeholder="Search for yesterday's most impactful AI news and send me a brief summary."
			></textarea>
		</div>

		<div class="flex flex-col gap-4">
			<div class="input-label text-base font-medium">Schedule</div>

			<div
				class={twMerge('grid gap-4', frequency === 'daily' ? 'md:grid-cols-2' : 'md:grid-cols-3')}
			>
				<div class="flex flex-col gap-2">
					<Select
						id="schedule-frequency"
						options={repeatOptions}
						selected={frequency}
						onSelect={(option) => {
							frequency = option.id as TaskFrequency;
							openSchedulePicker = null;
						}}
						class="border-base-300 min-h-12 border px-4 py-3 text-base"
					/>
				</div>

				{#if frequency === 'weekly'}
					<div class="relative w-full" bind:this={weeklyPickerEl}>
						<button
							type="button"
							class={twMerge(
								sectionInputClass(daysOfWeek.length === 0 && !!errorMessage),
								'flex w-full items-center justify-between gap-3'
							)}
							onclick={() => toggleSchedulePicker('weekly')}
						>
							<span
								class={twMerge(
									'truncate text-left',
									daysOfWeek.length === 0 && 'text-muted-content'
								)}
							>
								{schedulePickerButtonLabel('weekly')}
							</span>
							<ChevronDown
								class={twMerge(
									'size-5 shrink-0 transition-transform',
									openSchedulePicker === 'weekly' && 'rotate-180'
								)}
							/>
						</button>
						{#if openSchedulePicker === 'weekly'}
							<div class={pickerPanelClass()}>
								{#each weekdayOptions as option (option.value)}
									<button
										type="button"
										class="hover:bg-base-200 flex w-full items-center justify-between rounded-lg px-3 py-2.5 text-left text-sm transition-colors"
										onclick={() => toggleWeekday(option.value)}
									>
										<span>{option.label}</span>
										{#if daysOfWeek.includes(option.value)}
											<Check class="size-4" />
										{/if}
									</button>
								{/each}
							</div>
						{/if}
					</div>
				{:else if frequency === 'monthly'}
					<div class="relative w-full" bind:this={monthlyPickerEl}>
						<button
							type="button"
							class={twMerge(
								sectionInputClass(daysOfMonth.length === 0 && !!errorMessage),
								'flex w-full items-center justify-between gap-3'
							)}
							onclick={() => toggleSchedulePicker('monthly')}
						>
							<span
								class={twMerge(
									'truncate text-left',
									daysOfMonth.length === 0 && 'text-muted-content'
								)}
							>
								{schedulePickerButtonLabel('monthly')}
							</span>
							<ChevronDown
								class={twMerge(
									'size-5 shrink-0 transition-transform',
									openSchedulePicker === 'monthly' && 'rotate-180'
								)}
							/>
						</button>
						{#if openSchedulePicker === 'monthly'}
							<div class={pickerPanelClass()}>
								<div class="grid grid-cols-2 gap-1 sm:grid-cols-3">
									{#each monthDays as day (day)}
										<button
											type="button"
											class={twMerge(
												'hover:bg-base-200 flex items-center justify-between rounded-lg px-3 py-2.5 text-sm transition-colors',
												daysOfMonth.includes(day) && 'bg-base-200'
											)}
											onclick={() => toggleMonthDay(day)}
										>
											<span>{ordinal(day)}</span>
											{#if daysOfMonth.includes(day)}
												<Check class="size-4" />
											{/if}
										</button>
									{/each}
								</div>
							</div>
						{/if}
					</div>
				{:else if frequency === 'no_repeat'}
					<div class="flex flex-col gap-2">
						<DatePicker
							id="schedule-date"
							value={dateFromValue(date)}
							onChange={(selectedDate) => {
								date = valueFromDate(selectedDate);
							}}
							placeholder="Select date"
							format="MM-dd-yyyy"
							class="border-base-300 min-h-12 rounded-xl border px-4 py-3 text-base shadow-none"
						/>
					</div>
				{/if}

				<div class="flex flex-col gap-2">
					<TimeInput
						format={timePreference.timeFormat}
						date={timeAsDate}
						onChange={(d) => {
							time = `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
						}}
						class="border-base-300 h-12 gap-1 rounded-lg border text-base [&>div:nth-child(2)]:text-lg"
					/>
				</div>
			</div>
		</div>

		<div class="flex flex-col gap-3">
			<label for="schedule-expiration" class="input-label text-base font-medium">
				Expiration Date <span class="text-muted-content font-normal">(optional)</span>
			</label>
			<DatePicker
				id="schedule-expiration"
				value={dateFromValue(expiration)}
				onChange={(selectedDate) => {
					expiration = valueFromDate(selectedDate);
				}}
				placeholder={frequency === 'no_repeat'
					? `Defaults to ${formatScheduleDate(date) || 'the scheduled date'}`
					: 'No expiration'}
				format="MM-dd-yyyy"
				class="border-base-300 min-h-12 rounded-xl border px-4 py-3 text-base shadow-none"
			/>
			<p class="input-description">
				{#if frequency === 'no_repeat'}
					Leave empty to automatically expire this one-time schedule after {formatScheduleDate(
						date
					) || 'its scheduled date'}.
				{:else}
					Leave empty if this schedule should keep running until you disable it.
				{/if}
			</p>
		</div>

		<details class="border-base-300 rounded-2xl border">
			<summary
				class="flex cursor-pointer list-none items-center justify-between gap-3 px-5 py-4 text-base font-medium"
			>
				<span>Advanced Settings</span>
				<ChevronDown class="size-5 shrink-0 transition-transform" />
			</summary>
			<div class="border-base-300 border-t px-5 py-4">
				<div class="flex flex-wrap items-center justify-between gap-4">
					<div class="space-y-1">
						<div class="text-sm font-medium">Enabled</div>
						<p class="text-muted-content text-sm">
							Runs until it is disabled or reaches its expiration.
						</p>
					</div>
					<input type="checkbox" class="toggle" bind:checked={enabled} />
				</div>
				{#if currentTask}
					<div class="text-muted-content mt-4 text-sm">
						Timezone: <span class="text-base-content">{timezone}</span>
					</div>
				{/if}
			</div>
		</details>

		{#if errorMessage}
			<div class="alert alert-error rounded-2xl text-sm">
				<span>{errorMessage}</span>
			</div>
		{/if}

		<div class="flex justify-end gap-3">
			<button type="button" class="btn btn-ghost" onclick={() => dialog?.close()} disabled={saving}>
				Cancel
			</button>
			<button
				type="button"
				class="btn btn-primary min-w-32"
				onclick={handleSubmit}
				disabled={saving}
			>
				{#if saving}
					<Loading class="size-4" />
				{/if}
				Save
			</button>
		</div>
	</div>
</ResponsiveDialog>

<style lang="postcss">
	:global(details[open] > summary svg) {
		transform: rotate(180deg);
	}
</style>

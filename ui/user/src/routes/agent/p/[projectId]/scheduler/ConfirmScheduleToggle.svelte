<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import { estimateNextRun, formatScheduleDateTime } from '$lib/components/nanobot/taskSchedule';
	import { m } from '$lib/i18n';
	import { userDeviceSettings } from '$lib/stores';

	interface Props {
		task?: {
			uri: string;
			name: string;
			enabled?: boolean;
			schedule?: string;
			expiration?: string;
		};
		onSuccess: () => void;
		onCancel: () => void;
		loading: boolean;
	}

	let { task, onSuccess, onCancel, loading }: Props = $props();
</script>

{#if task}
	<Confirm
		show={!!task}
		title={task.enabled ? m.chat_confirm_disable() : m.chat_confirm_enable()}
		msg={task.enabled
			? m.chat_disable_named({ name: task.name })
			: m.chat_enable_named({ name: task.name })}
		{loading}
		onsuccess={onSuccess}
		oncancel={onCancel}
		type="info"
	>
		{#snippet note()}
			{#if task}
				{@const nextRun = task?.schedule
					? estimateNextRun(task.schedule, task.expiration)
					: undefined}

				{#if task}
					{#if task.enabled}
						<p>{m.chat_schedule_disable_note()}</p>
						<p class="mt-2">{m.chat_schedule_disable_confirm()}</p>
					{:else if !task.enabled && nextRun}
						<p>{m.chat_schedule_next_run_at()}</p>
						<p class="mt-2 font-semibold">
							{formatScheduleDateTime(nextRun.toISOString(), userDeviceSettings.timeFormat)}
						</p>
						<p class="mt-2">{m.chat_schedule_enable_confirm()}</p>
					{/if}
				{/if}
			{/if}
		{/snippet}
	</Confirm>
{/if}

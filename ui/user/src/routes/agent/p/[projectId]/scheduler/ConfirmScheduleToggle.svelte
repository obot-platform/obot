<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import { estimateNextRun, formatScheduleDateTime } from '$lib/components/nanobot/taskSchedule';
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
		title={task.enabled ? 'Confirm Disable' : 'Confirm Enable'}
		msg={task.enabled ? `Disable ${task.name}?` : `Enable ${task.name}?`}
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
						<p>All upcoming runs will not be performed until this task is re-enabled.</p>
						<p class="mt-2">Are you sure you want to disable this schedule?</p>
					{:else if !task.enabled && nextRun}
						<p>The next run will be executed at:</p>
						<p class="mt-2 font-semibold">
							{formatScheduleDateTime(nextRun.toISOString(), userDeviceSettings.timeFormat)}
						</p>
						<p class="mt-2">Are you sure you want to enable this schedule?</p>
					{/if}
				{/if}
			{/if}
		{/snippet}
	</Confirm>
{/if}

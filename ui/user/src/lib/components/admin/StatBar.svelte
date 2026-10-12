<script lang="ts">
	import { m } from '$lib/i18n';
	import { formatTimeRange } from '$lib/time';

	interface Props {
		totalCalls?: number;
		uniqueUsers?: number;
		startTime?: string;
		endTime?: string;
	}

	let { totalCalls, uniqueUsers, startTime, endTime }: Props = $props();
</script>

{#if startTime && endTime}
	<div
		class="dark:bg-base-400 dark:border-base-400 bg-base-100 flex h-12 w-fit flex-wrap gap-4 rounded-md border border-transparent px-6 py-2 shadow-sm"
	>
		<p class="flex items-center text-sm font-semibold">
			{formatTimeRange(startTime, endTime)}
		</p>
	</div>
{/if}

{#if totalCalls || uniqueUsers}
	<div
		class="dark:bg-base-400 dark:border-base-400 bg-base-100 flex h-12 w-fit flex-wrap gap-4 rounded-md border border-transparent px-6 py-2 shadow-sm"
	>
		{#if totalCalls}
			<div class="flex items-center justify-between gap-4 text-sm">
				<p class="font-semibold">{m.core_total_calls()}</p>
				<p>
					{totalCalls}
				</p>
			</div>
		{/if}
		{#if totalCalls && uniqueUsers}
			<div class="bg-base-400 h-8 w-0.5"></div>
		{/if}
		{#if uniqueUsers}
			<div class="flex items-center justify-between gap-4 text-sm">
				<p class="font-semibold">{m.core_unique_users()}</p>
				<p>
					{uniqueUsers}
				</p>
			</div>
		{/if}
	</div>
{/if}

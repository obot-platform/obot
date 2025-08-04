<script lang="ts">
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
		class="dark:bg-surface2 dark:border-surface3 flex h-12 w-fit flex-wrap gap-4 rounded-md border border-transparent bg-white px-6 py-2 text-black shadow-sm dark:text-white"
	>
		<p class="flex items-center text-sm font-semibold">
			{formatTimeRange(startTime, endTime)}
		</p>
	</div>
{/if}

{#if totalCalls || uniqueUsers}
	<div
		class="dark:bg-surface2 dark:border-surface3 flex h-12 w-fit flex-wrap gap-4 rounded-md border border-transparent bg-white px-6 py-2 text-black shadow-sm dark:text-white"
	>
		{#if totalCalls}
			<div class="flex items-center justify-between gap-4 text-sm">
				<p class="font-semibold">Total Calls:</p>
				<p>
					{totalCalls}
				</p>
			</div>
		{/if}
		{#if totalCalls && uniqueUsers}
			<div class="bg-surface3 h-8 w-0.5"></div>
		{/if}
		{#if uniqueUsers}
			<div class="flex items-center justify-between gap-4 text-sm">
				<p class="font-semibold">Unique Users:</p>
				<p>
					{uniqueUsers}
				</p>
			</div>
		{/if}
	</div>
{/if}

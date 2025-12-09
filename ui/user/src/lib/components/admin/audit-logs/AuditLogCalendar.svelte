<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Calendar from '$lib/components/Calendar.svelte';
	import { formatTimeRange, getTimeRangeShorthand } from '$lib/time';
	import { set, startOfDay, subDays, subHours } from 'date-fns';
	import { twMerge } from 'tailwind-merge';
	import { clickOutsidePopoverContent, Popover } from '$lib/components/ui/popover';

	let { start, end, disabled = false, onChange } = $props();

	let isQuickActionOpen = $state(false);

	const actions = [
		{
			label: 'Last Hour',
			onpointerdown: () => {
				end = set(new Date(), { milliseconds: 0, seconds: 59 });

				start = subHours(end, 1);

				onChange({ end: end, start });
				isQuickActionOpen = false;
			}
		},
		{
			label: 'Last 6 Hour',
			onpointerdown: () => {
				end = set(new Date(), { milliseconds: 0, seconds: 59 });
				start = subHours(end, 6);

				onChange({ end, start });
				isQuickActionOpen = false;
			}
		},
		{
			label: 'Last 24 Hour',
			onpointerdown: () => {
				end = set(new Date(), { milliseconds: 0, seconds: 59 });
				start = subHours(end, 24);

				onChange({ end, start });
				isQuickActionOpen = false;
			}
		},
		{
			label: 'Last 7 Days',
			onpointerdown: () => {
				end = set(new Date(), { milliseconds: 0, seconds: 59 });
				start = startOfDay(subDays(end, 7));

				onChange({ end, start: start });
				isQuickActionOpen = false;
			}
		},
		{
			label: 'Last 30 Days',
			onpointerdown: () => {
				end = set(new Date(), { milliseconds: 0, seconds: 59 });
				start = startOfDay(subDays(end, 30));

				onChange({ end, start: start });
				isQuickActionOpen = false;
			}
		},
		{
			label: 'Last 60 Days',
			onpointerdown: () => {
				end = set(new Date(), { milliseconds: 0, seconds: 59 });
				start = startOfDay(subDays(end, 60));

				onChange({ end, start: start });
				isQuickActionOpen = false;
			}
		},
		{
			label: 'Last 90 Days',
			onpointerdown: () => {
				end = set(new Date(), { milliseconds: 0, seconds: 59 });
				start = startOfDay(subDays(end, 90));

				onChange({ end, start: start });
				isQuickActionOpen = false;
			}
		}
	];
</script>

<div class="flex">
	<Popover.Root bind:open={isQuickActionOpen} placement="bottom-start" offset={4}>
		<Popover.Trigger
			type="button"
			class="dark:border-surface3 dark:hover:bg-surface2/70 dark:active:bg-surface2 dark:bg-surface1 hover:bg-surface1/70 active:bg-surface1 bg-background flex min-h-12.5 flex-shrink-0 items-center gap-2 truncate rounded-l-lg border border-r-0 border-transparent px-2 text-sm shadow-sm transition-colors duration-200 disabled:opacity-50"
			{disabled}
			{@attach (node: HTMLElement) => {
				const response = tooltip(node, {
					text: 'Calendar Quick Actions',
					placement: 'top-end'
				});

				return () => response.destroy();
			}}
		>
			<span class="bg-surface3 rounded-md px-3 py-1 text-xs">
				{getTimeRangeShorthand(start, end)}
			</span>
			<span>
				{formatTimeRange(start, end)}
			</span>
		</Popover.Trigger>

		<Popover.Content
			class={twMerge('default-dialog flex flex-col items-start p-0')}
			{@attach clickOutsidePopoverContent(() => (isQuickActionOpen = false))}
		>
			{#each actions as action (action.label)}
				<button
					class="hover:bg-surface3 w-full min-w-max px-4 py-2 text-start"
					onpointerdown={action.onpointerdown}
				>
					{action.label}
				</button>
			{/each}
		</Popover.Content>
	</Popover.Root>

	<Calendar
		compact
		class="dark:border-surface3 hover:bg-surface1 dark:hover:bg-surface3 dark:bg-surface1 bg-background flex min-h-12.5 flex-shrink-0 items-center gap-2 truncate rounded-none rounded-r-lg border border-transparent px-4 text-sm shadow-sm"
		initialValue={{
			start: new Date(start),
			end: end ? new Date(end) : null
		}}
		{start}
		{end}
		{disabled}
		{onChange}
	/>
</div>

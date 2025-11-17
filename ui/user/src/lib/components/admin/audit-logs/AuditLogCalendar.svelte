<script lang="ts">
	import { clickOutside } from '$lib/actions/clickoutside';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Calendar from '$lib/components/Calendar.svelte';
	import { formatTimeRange, getTimeRangeShorthand } from '$lib/time';
	import { set, startOfDay, subDays, subHours } from 'date-fns';
	import { twMerge } from 'tailwind-merge';

	let { start, end, disabled = false, onChange } = $props();

	let quickAccessPopover = $state<HTMLDialogElement>();

	const actions = [
		{
			label: 'Last Hour',
			onpointerdown: () => {
				end = set(new Date(), { milliseconds: 0, seconds: 59 });

				start = subHours(end, 1);

				onChange({ end: end, start });
				quickAccessPopover?.close();
			}
		},
		{
			label: 'Last 6 Hour',
			onpointerdown: () => {
				end = set(new Date(), { milliseconds: 0, seconds: 59 });
				start = subHours(end, 6);

				onChange({ end, start });
				quickAccessPopover?.close();
			}
		},
		{
			label: 'Last 24 Hour',
			onpointerdown: () => {
				end = set(new Date(), { milliseconds: 0, seconds: 59 });
				start = subHours(end, 24);

				onChange({ end, start });
				quickAccessPopover?.close();
			}
		},
		{
			label: 'Last 7 Days',
			onpointerdown: () => {
				end = set(new Date(), { milliseconds: 0, seconds: 59 });
				start = startOfDay(subDays(end, 7));

				onChange({ end, start: start });
				quickAccessPopover?.close();
			}
		},
		{
			label: 'Last 30 Days',
			onpointerdown: () => {
				end = set(new Date(), { milliseconds: 0, seconds: 59 });
				start = startOfDay(subDays(end, 30));

				onChange({ end, start: start });
				quickAccessPopover?.close();
			}
		},
		{
			label: 'Last 60 Days',
			onpointerdown: () => {
				end = set(new Date(), { milliseconds: 0, seconds: 59 });
				start = startOfDay(subDays(end, 60));

				onChange({ end, start: start });
				quickAccessPopover?.close();
			}
		},
		{
			label: 'Last 90 Days',
			onpointerdown: () => {
				end = set(new Date(), { milliseconds: 0, seconds: 59 });
				start = startOfDay(subDays(end, 90));

				onChange({ end, start: start });
				quickAccessPopover?.close();
			}
		}
	];
</script>

<div class="flex">
	<div class="relative flex items-center">
		<button
			type="button"
			class="dark:border-surface3 dark:hover:bg-surface2/70 dark:active:bg-surface2 dark:bg-surface1 hover:bg-surface1/70 active:bg-surface1 bg-background flex min-h-12.5 flex-shrink-0 items-center gap-2 truncate rounded-l-lg border border-r-0 border-transparent px-2 text-sm shadow-sm transition-colors duration-200 disabled:opacity-50"
			{disabled}
			onpointerdown={() => {
				if (quickAccessPopover?.open) {
					quickAccessPopover?.close();
				} else {
					quickAccessPopover?.show();
				}
			}}
			use:tooltip={{
				text: 'Calendar Quick Actions',
				placement: 'top-end'
			}}
		>
			<span class="bg-surface3 rounded-md px-3 py-1 text-xs">
				{getTimeRangeShorthand(start, end)}
			</span>
			<span>
				{formatTimeRange(start, end)}
			</span>
		</button>

		<dialog
			use:clickOutside={[() => quickAccessPopover?.close(), true]}
			class={twMerge(
				'p-y absolute top-full right-0 left-[unset] z-50 m-0 mt-1 min-w-fit overflow-hidden'
			)}
			{@attach (node) => node.close()}
			{@attach (node) => (quickAccessPopover = node)}
		>
			<div class="flex flex-col items-start">
				{#each actions as action (action.label)}
					<button
						class="hover:bg-surface3 w-full min-w-max px-4 py-2 text-start"
						onpointerdown={action.onpointerdown}
					>
						{action.label}
					</button>
				{/each}
			</div>
		</dialog>
	</div>

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

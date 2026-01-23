<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import popover from '$lib/actions/popover.svelte';
	import Calendar from '$lib/components/Calendar.svelte';
	import { formatTimeRange, getTimeRangeShorthand } from '$lib/time';
	import { set, startOfDay, subDays, subHours } from 'date-fns';
	import { twMerge } from 'tailwind-merge';
	import { responsive } from '$lib/stores';

	let { start, end, disabled = false, onChange } = $props();

	let open = $state(false);

	const actions = [
		{
			label: 'Last Hour',
			onpointerdown: () => {
				end = set(new Date(), { milliseconds: 0, seconds: 59 });

				start = subHours(end, 1);

				onChange({ end: end, start });
				quickActionsPopover.toggle(false);
			}
		},
		{
			label: 'Last 6 Hour',
			onpointerdown: () => {
				end = set(new Date(), { milliseconds: 0, seconds: 59 });
				start = subHours(end, 6);

				onChange({ end, start });
				quickActionsPopover.toggle(false);
			}
		},
		{
			label: 'Last 24 Hour',
			onpointerdown: () => {
				end = set(new Date(), { milliseconds: 0, seconds: 59 });
				start = subHours(end, 24);

				onChange({ end, start });
				quickActionsPopover.toggle(false);
			}
		},
		{
			label: 'Last 7 Days',
			onpointerdown: () => {
				end = set(new Date(), { milliseconds: 0, seconds: 59 });
				start = startOfDay(subDays(end, 7));

				onChange({ end, start: start });
				quickActionsPopover.toggle(false);
			}
		},
		{
			label: 'Last 30 Days',
			onpointerdown: () => {
				end = set(new Date(), { milliseconds: 0, seconds: 59 });
				start = startOfDay(subDays(end, 30));

				onChange({ end, start: start });
				quickActionsPopover.toggle(false);
			}
		},
		{
			label: 'Last 60 Days',
			onpointerdown: () => {
				end = set(new Date(), { milliseconds: 0, seconds: 59 });
				start = startOfDay(subDays(end, 60));

				onChange({ end, start: start });
				quickActionsPopover.toggle(false);
			}
		},
		{
			label: 'Last 90 Days',
			onpointerdown: () => {
				end = set(new Date(), { milliseconds: 0, seconds: 59 });
				start = startOfDay(subDays(end, 90));

				onChange({ end, start: start });
				quickActionsPopover.toggle(false);
			}
		}
	];

	const quickActionsPopover = popover({
		placement: 'bottom-start',
		offset: 4,
		onOpenChange: (val) => {
			open = val;
		}
	});

	const isSmallScreen = $derived(responsive.isMobile);

	function refAction(node: HTMLElement) {
		return quickActionsPopover.ref(node);
	}

	function tooltipAction(node: HTMLElement) {
		if (isSmallScreen) {
			return;
		}

		return quickActionsPopover.tooltip(node);
	}
</script>

<div class="flex w-full md:max-w-fit">
	<button
		type="button"
		class="dark:border-surface3 dark:hover:bg-surface2/70 dark:active:bg-surface2 dark:bg-surface1 hover:bg-surface1/70 active:bg-surface1 bg-background relative z-40 flex min-h-12.5 flex-1 flex-shrink-0 items-center gap-2 truncate rounded-l-lg border border-r-0 border-transparent px-2 text-sm shadow-sm transition-colors duration-200 disabled:opacity-50"
		{disabled}
		use:refAction
		onclick={() => {
			if (!disabled) {
				quickActionsPopover.toggle();
			}
		}}
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
	</button>

	<div
		class={twMerge(
			isSmallScreen
				? 'fixed inset-0 z-50 flex min-w-full items-center justify-center p-4 backdrop-blur-sm'
				: 'contents',
			!open && 'hidden'
		)}
		role="button"
		tabindex="-1"
		onclick={(ev) => {
			if (
				ev.target &&
				ev.currentTarget.contains(ev.target as HTMLElement) &&
				!(ev.currentTarget === ev.target)
			) {
				return;
			}
			quickActionsPopover.toggle(false);
		}}
		onkeydown={undefined}
	>
		{#key isSmallScreen}
			<div class="popover flex w-full max-w-sm flex-col py-2 md:max-w-fit" use:tooltipAction>
				<div class="mb-6 px-4 text-center text-lg font-medium md:hidden md:text-start">
					<div>Select Export Time Range</div>
				</div>

				<div class="flex w-full min-w-36 flex-col">
					{#each actions as action (action.label)}
						<button
							type="button"
							class="hover:bg-surface3/25 h-12 w-full min-w-max px-4 py-2 text-center last:border-b-transparent md:h-10 md:text-start"
							onclick={action.onpointerdown}
						>
							{action.label}
						</button>
					{/each}
				</div>
			</div>
		{/key}
	</div>

	<Calendar
		compact
		class="dark:border-surface3 hover:bg-surface1 dark:hover:bg-surface3 dark:bg-surface1 bg-background relative z-40 flex min-h-12.5 flex-shrink-0 items-center gap-2 truncate rounded-none rounded-r-lg border border-transparent px-4 text-sm shadow-sm"
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

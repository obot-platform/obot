<script lang="ts">
	import { fade } from 'svelte/transition';
	import { GlobeIcon } from 'lucide-svelte';
	import type { Schedule } from '$lib/services';
	import Dropdown from '$lib/components/tasks/Dropdown.svelte';
	import Combobox from './Combobox.svelte';
	import AmPmSwith from './AmPmSwith.svelte';

	interface Props {
		schedule?: Schedule;
		readOnly?: boolean;
	}

	let { schedule = $bindable(), readOnly }: Props = $props();

	let defaultTimezone = $state(Intl.DateTimeFormat().resolvedOptions().timeZone);

	const hourlyValues: Record<string, string> = {
		'0': 'on the hour',
		'15': '15 minutes past',
		'30': '30 minutes past',
		'45': '45 minutes past'
	};

	let hourlyComboboxFocused = $state(false);

	const dailyValues: Record<string, string> = {
		'0': 'midnight',
		'3': '3 AM',
		'6': '6 AM',
		'9': '9 AM',
		'12': 'noon',
		'15': '3 PM',
		'18': '6 PM',
		'21': '9 PM'
	};

	let dailyComboboxFocused = $state(false);
	let dailyComboboxAmPm: 'am' | 'pm' = $state('am');

	function to12H(value: number) {
		if (value === 0 || value > 23) {
			return 0;
		}

		if (value === 12) {
			return 12;
		}

		// value in 24H format
		if (value > 12) {
			return Math.min(11, Math.max(0, Math.trunc(value - 12)));
		}

		// value in 12H AM, just make sure it is within the boudaries
		return Math.min(11, Math.max(0, value));
	}

	function from12H(value: number, ampm: 'am' | 'pm' = 'am') {
		let v = value;
		let factor = 0;

		// if value is greateer than 11; subtract 12 hours to make it 12H format
		if (value > 11) {
			v -= 12;
		}

		// if pm is selected then add 12 hours to make it 24H format
		if (ampm === 'pm') {
			factor = 12;
		}

		// sum and check for boundaries
		return Math.trunc(Math.min(23, Math.max(0, v + factor)));
	}
</script>

<div class="flex h-12 items-center self-start">
	<h4 class="text-base font-medium">Schedule</h4>
</div>
<div class="flex min-w-[220px] flex-col gap-4 md:min-w-auto lg:flex-row">
	<Dropdown
		class="schedule-dropdown w-full md:w-[172px]"
		values={{
			hourly: 'hourly',
			daily: 'daily',
			weekly: 'weekly',
			monthly: 'monthly'
		}}
		selected={schedule?.interval}
		onSelected={(value) => {
			if (schedule) {
				schedule.interval = value;
			}
		}}
		disabled={readOnly}
	/>

	{#if schedule?.interval === 'hourly'}
		<Combobox
			class="schedule-dropdown w-full overflow-hidden md:w-[220px]"
			type="number"
			values={hourlyValues}
			value={schedule?.minute.toString()}
			disabled={readOnly}
			onselect={(value) => {
				if (schedule) {
					schedule.minute = Math.min(60, Math.max(0, parseInt(value)));
				}
			}}
			onblur={() => {
				setTimeout(() => {
					hourlyComboboxFocused = false;
				}, 100);
			}}
			onfocus={() => {
				hourlyComboboxFocused = true;
			}}
		>
			{#if !hourlyComboboxFocused && schedule?.minute === 0}
				{@const key = schedule?.minute?.toString() ?? ''}
				<div
					class="pointer-events-none absolute inset-0 flex items-center pl-4 backdrop-blur-2xl"
					transition:fade={{ duration: 100 }}
				>
					<div>{hourlyValues[key]}</div>
				</div>
			{/if}
		</Combobox>
	{/if}

	{#if schedule?.interval === 'daily'}
		<Combobox
			class="schedule-dropdown w-full overflow-hidden md:w-[220px]"
			type="number"
			values={dailyValues}
			value={to12H(schedule?.hour ?? 0).toString()}
			selected={(schedule?.hour ?? 0).toString()}
			disabled={readOnly}
			onselect={(value) => {
				if (schedule) {
					const valueAsInt = parseInt(value);
					dailyComboboxAmPm = valueAsInt > 11 ? 'pm' : 'am';

					const hour = from12H(valueAsInt, dailyComboboxAmPm);
					schedule.hour = hour;
				}
			}}
			onfocus={() => {
				dailyComboboxFocused = true;
			}}
			onclickout={(ev) => {
				const target = ev.target as HTMLElement;

				const comboboxRootElement = target.closest('.combobox');

				// click target is inside combobox element
				if (comboboxRootElement) {
					return;
				}

				// click target is outside combobox element
				dailyComboboxFocused = false;
			}}
		>
			{#snippet post()}
				<div class="border-surface3 border-x pr-0" role="button" tabindex="0" onkeydown={() => {}}>
					<AmPmSwith
						bind:value={dailyComboboxAmPm}
						onchange={(value) => {
							const hour = from12H(schedule.hour, value);
							schedule.hour = hour;
						}}
						onclick={(ev) => {
							ev.stopPropagation();

							if (!dailyComboboxFocused) {
								dailyComboboxFocused = true;
							}
						}}
					/>
				</div>
			{/snippet}

			{#if !dailyComboboxFocused}
				{@const key = schedule?.hour?.toString() ?? ''}
				<div
					class="pointer-events-none absolute inset-0 flex items-center pl-4 backdrop-blur-[80px]"
					transition:fade={{ duration: 50 }}
				>
					{#if schedule?.hour === 0 || schedule?.hour === 12}
						<div>{dailyValues[key]}</div>
					{:else}
						<div>
							<span>{schedule?.hour ?? 0}</span>
							<span class="uppercase">{dailyComboboxAmPm}</span>
						</div>
					{/if}
				</div>
			{/if}
		</Combobox>
		{#if schedule?.timezone && schedule.timezone !== defaultTimezone}
			<div class="flex items-center gap-1">
				<GlobeIcon class="text-muted-foreground mr-1 h-4 w-4" />
				<span class="text-muted-foreground text-sm">{schedule.timezone}</span>
			</div>
		{/if}
	{/if}

	{#if schedule?.interval === 'weekly'}
		<Dropdown
			class="schedule-dropdown w-full md:w-[220px]"
			values={{
				'0': 'Sunday',
				'1': 'Monday',
				'2': 'Tuesday',
				'3': 'Wednesday',
				'4': 'Thursday',
				'5': 'Friday',
				'6': 'Saturday'
			}}
			selected={schedule?.weekday.toString()}
			onSelected={(value) => {
				if (schedule) {
					schedule.weekday = parseInt(value);
				}
			}}
			disabled={readOnly}
		/>
		{#if schedule?.timezone && schedule.timezone !== defaultTimezone}
			<div class="flex items-center gap-1">
				<GlobeIcon class="text-muted-foreground mr-1 h-4 w-4" />
				<span class="text-muted-foreground text-sm">{schedule.timezone}</span>
			</div>
		{/if}
	{/if}

	{#if schedule?.interval === 'monthly'}
		<!-- The value of schedule.day is saved on the backend based on zero-based numbering, whereas the user is familliar with one-based numbering -->
		<!-- The solution will be to convert back and forth -->

		{@const day = schedule?.day ?? 0}

		<Combobox
			class="schedule-dropdown w-full md:w-[220px]"
			type="number"
			values={{
				'1': '1st',
				'2': '2nd',
				'3': '3rd',
				'5': '5th',
				'15': '15th',
				'20': '20th',
				'25': '25th',
				'-1': 'last day'
			}}
			value={(day >= 0 ? day + 1 : day).toString()}
			onselect={(value) => {
				if (schedule) {
					const valueAsNumber = parseInt(value);

					if (valueAsNumber >= 0) {
						// reset counting to 0-based
						schedule.day = Math.min(31, Math.max(0, valueAsNumber - 1));
					} else {
						// negative set as is
						schedule.day = valueAsNumber;
					}
				}
			}}
			disabled={readOnly}
		/>
		{#if schedule?.timezone && schedule.timezone !== defaultTimezone}
			<div class="flex items-center gap-1">
				<GlobeIcon class="text-muted-foreground mr-1 h-4 w-4" />
				<span class="text-muted-foreground text-sm">{schedule.timezone}</span>
			</div>
		{/if}
	{/if}
</div>

<style lang="postcss">
	:global(.schedule-dropdown) {
		background-color: var(--surface2);
		font-size: var(--text-md);
		display: flex;
		flex-grow: 1;
	}
</style>

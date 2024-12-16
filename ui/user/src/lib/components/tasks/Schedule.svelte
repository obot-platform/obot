<script lang="ts">
	import type { Schedule } from '$lib/services';
	import Dropdown from '$lib/components/tasks/Dropdown.svelte';

	interface Props {
		schedule?: Schedule;
		editMode?: boolean;
		onChanged?: (schedule: Schedule) => void | Promise<void>;
	}

	let {
		editMode = false,
		schedule = {
			interval: '',
			hour: 0,
			minute: 0,
			day: 0,
			weekday: 0
		},
		onChanged
	}: Props = $props();
</script>

<h3 class="text-lg font-semibold">Schedule</h3>
<div class="flex">
	<Dropdown
		values={{
			hourly: 'hourly',
			daily: 'daily',
			weekly: 'weekly',
			monthly: 'monthly'
		}}
		selected={schedule?.interval}
		disabled={!editMode}
		onSelected={(value) => {
			onChanged?.({
				...schedule,
				interval: value
			});
		}}
	/>

	{#if schedule.interval === 'hourly'}
		<Dropdown
			values={{
				'0': 'on the hour',
				'15': '15 minutes past',
				'30': '30 minutes past',
				'45': '45 minutes past'
			}}
			selected={schedule?.minute.toString()}
			disabled={!editMode}
			onSelected={(value) => {
				onChanged?.({
					...schedule,
					minute: parseInt(value),
					hour: 0,
					day: 0,
					weekday: 0
				});
			}}
		/>
	{/if}

	{#if schedule.interval === 'daily'}
		<Dropdown
			values={{
				'0': 'midnight',
				'3': '3 AM',
				'6': '6 AM',
				'9': '9 AM',
				'12': 'noon',
				'15': '3 PM',
				'18': '6 PM',
				'21': '9 PM'
			}}
			selected={schedule?.hour.toString()}
			disabled={!editMode}
			onSelected={(value) => {
				onChanged?.({
					...schedule,
					minute: 0,
					hour: parseInt(value),
					day: 0,
					weekday: 0
				});
			}}
		/>
	{/if}

	{#if schedule.interval === 'weekly'}
		<Dropdown
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
			disabled={!editMode}
			onSelected={(value) => {
				onChanged?.({
					...schedule,
					minute: 0,
					hour: 0,
					day: 0,
					weekday: parseInt(value)
				});
			}}
		/>
	{/if}

	{#if schedule.interval === 'monthly'}
		<Dropdown
			values={{
				'0': '1st',
				'1': '2nd',
				'2': '3rd',
				'4': '5th',
				'14': '15th',
				'19': '20th',
				'24': '25th',
				'-1': 'last day'
			}}
			selected={schedule?.day.toString()}
			disabled={!editMode}
			onSelected={(value) => {
				onChanged?.({
					...schedule,
					minute: 0,
					hour: 0,
					day: parseInt(value),
					weekday: 0
				});
			}}
		/>
	{/if}
</div>

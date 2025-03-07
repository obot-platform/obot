<script lang="ts">
	import type { Schedule } from '$lib/services';
	import Dropdown from '$lib/components/tasks/Dropdown.svelte';

	interface Props {
		schedule?: Schedule;
	}

	let { schedule = $bindable() }: Props = $props();
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
		onSelected={(value) => {
			if (schedule) {
				schedule.interval = value;
			}
		}}
	/>

	{#if schedule?.interval === 'hourly'}
		<Dropdown
			values={{
				'0': 'on the hour',
				'15': '15 minutes past',
				'30': '30 minutes past',
				'45': '45 minutes past'
			}}
			selected={schedule?.minute.toString()}
			onSelected={(value) => {
				if (schedule) {
					schedule.minute = parseInt(value);
				}
			}}
		/>
	{/if}

	{#if schedule?.interval === 'daily'}
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
			onSelected={(value) => {
				if (schedule) {
					schedule.hour = parseInt(value);
				}
			}}
		/>
	{/if}

	{#if schedule?.interval === 'weekly'}
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
			onSelected={(value) => {
				if (schedule) {
					schedule.weekday = parseInt(value);
				}
			}}
		/>
	{/if}

	{#if schedule?.interval === 'monthly'}
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
			onSelected={(value) => {
				if (schedule) {
					schedule.day = parseInt(value);
				}
			}}
		/>
	{/if}
</div>

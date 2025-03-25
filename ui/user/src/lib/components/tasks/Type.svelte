<script lang="ts">
	import { type Task } from '$lib/services';
	import Dropdown from '$lib/components/tasks/Dropdown.svelte';
	import { version } from '$lib/stores';
	import Trigger from './Trigger.svelte';

	interface Props {
		task?: Task;
		readOnly?: boolean;
	}

	let { task = $bindable(), readOnly }: Props = $props();
	let options = $derived.by(() => {
		const options: Record<string, string> = {
			onDemand: 'on demand',
			schedule: 'on interval',
			webhook: 'on webhook'
		};
		if (version.current.emailDomain) {
			options['email'] = 'on email';
		}
		// assigned later so it's rendered last
		options['onDemand'] = 'on demand';
		return options;
	});

	function selectedTrigger(): string {
		if (task?.schedule) {
			return 'schedule';
		}
		if (task?.webhook) {
			return 'webhook';
		}
		if (task?.email) {
			return 'email';
		}
		return 'onDemand';
	}

	async function selected(value: string) {
		if (!task) {
			return;
		}
		if (value === 'schedule') {
			task.schedule = {
				interval: 'hourly',
				hour: 0,
				minute: 0,
				day: 0,
				weekday: 0
			};
			task.webhook = undefined;
			task.email = undefined;
			task.onDemand = undefined;
		}
		if (value === 'webhook') {
			task.schedule = undefined;
			task.webhook = {};
			task.email = undefined;
			task.onDemand = undefined;
		}
		if (value === 'email') {
			task.schedule = undefined;
			task.webhook = undefined;
			task.onDemand = undefined;
			task.email = {};
		}
		if (value === 'onDemand') {
			task.schedule = undefined;
			task.webhook = undefined;
			task.email = undefined;
			task.onDemand = undefined;
		}
	}
</script>

<div class="flex grow flex-col rounded-2xl bg-gray-50 p-5 dark:bg-gray-950">
	<div class="border-surface3 mb-4 flex items-center justify-between gap-4 border-b pb-4">
		<h3 class="text-lg font-semibold">Trigger Type</h3>
		<Dropdown
			class="bg-surface2 md:min-w-sm"
			selected={selectedTrigger()}
			values={options}
			onSelected={selected}
			disabled={readOnly}
		/>
	</div>

	{#if task}
		<Trigger bind:task {readOnly} />
	{/if}
</div>

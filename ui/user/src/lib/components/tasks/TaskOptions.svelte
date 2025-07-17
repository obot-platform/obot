<script lang="ts">
	import { type Task } from '$lib/services';
	import Dropdown from '$lib/components/tasks/Dropdown.svelte';
	import Trigger from './Trigger.svelte';

	interface Props {
		task?: Task;
		readOnly?: boolean;
	}

	let { task = $bindable(), readOnly }: Props = $props();

	let options = {
		schedule: 'on interval',
		onDemand: 'on demand'
	};

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
		if (task?.onSlackMessage) {
			return 'slack';
		}
		if (task?.onDiscordMessage) {
			return 'discord';
		}
		return 'onDemand';
	}

	let triggerType = $derived(selectedTrigger());

	async function selected(value: string) {
		if (!task) {
			return;
		}

		if (value === 'schedule') {
			task.schedule = {
				interval: 'daily',
				hour: 0,
				minute: 0,
				day: 0,
				weekday: 0,
				timezone: Intl.DateTimeFormat().resolvedOptions().timeZone
			};
			task.webhook = undefined;
			task.email = undefined;
			task.onDemand = undefined;
			task.onSlackMessage = undefined;
		}

		if (value === 'onDemand') {
			task.schedule = undefined;
			task.webhook = undefined;
			task.email = undefined;
			task.onDemand = undefined;
			task.onSlackMessage = undefined;
		}
	}
</script>

{#if !readOnly}
	<div
		class="dark:bg-surface1 dark:border-surface3 flex grow flex-col overflow-visible rounded-lg bg-white p-5 shadow-sm dark:border"
	>
		{#if triggerType === 'onDemand' || triggerType === 'schedule'}
			<div class="border-surface3 mb-4 flex flex-col gap-4 border-b pb-4">
				<div
					class="flex w-full flex-col justify-start gap-4 lg:flex-row lg:items-center lg:justify-between"
				>
					<h3 class="text-lg font-semibold">How do you want to run this task?</h3>
					<Dropdown
						class="bg-surface2 xl:min-w-sm"
						selected={selectedTrigger()}
						values={options}
						onSelected={selected}
						disabled={readOnly}
					/>
				</div>
				<p class="text-gray text-sm">
					{#if triggerType === 'onDemand'}
						On demands tasks can be ran manually from the UI or invoked by your project from chat
						threads or even other tasks. Just tell it to invoke them by name like this: “Call the
						Webpage Summarizer task.”
					{:else if triggerType === 'schedule'}
						Scheduled tasks will be ran autonomously on your specified interval. Like on demand
						tasks, they can also be invoked from the UI or by your project, but you cannot add
						arguments to a scheduled task.
					{/if}
				</p>
			</div>
		{/if}

		{#if task}
			<Trigger bind:task {readOnly} />
		{/if}
	</div>
{/if}

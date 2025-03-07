<script lang="ts">
	import { ChatService, type Task, type Version } from '$lib/services';
	import { onMount } from 'svelte';
	import Dropdown from '$lib/components/tasks/Dropdown.svelte';

	interface Props {
		task?: Task;
	}

	let { task = $bindable() }: Props = $props();

	let inputVisible = $derived.by(() => {
		if (task?.webhook || task?.schedule || task?.email) {
			return false;
		}
		return Object.keys(task?.onDemand?.params ?? {}).length === 0;
	});
	let version: Version = $state({});
	let options = $derived.by(() => {
		const options: Record<string, string> = {
			onDemand: 'on demand',
			schedule: 'on interval',
			webhook: 'on webhook'
		};
		if (version.emailDomain) {
			options['email'] = 'on email';
		}
		// assigned later so it's rendered last
		options['onDemand'] = 'on demand';
		return options;
	});

	onMount(async () => {
		version = await ChatService.getVersion();
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

<div class="flex flex-1 justify-end">
	<div class="flex items-center">
		<button
			class="ml-2 flex items-center rounded-3xl p-2 px-4 text-gray hover:bg-gray-70 hover:text-black dark:hover:bg-gray-900 dark:hover:text-gray-50"
			class:hidden={!inputVisible}
			onclick={() => {
				if (!task) {
					return;
				}
				task.onDemand = {
					params: { '': '' }
				};
			}}
		>
			Add Arguments
		</button>
	</div>
	<Dropdown selected={selectedTrigger()} values={options} onSelected={selected} />
</div>

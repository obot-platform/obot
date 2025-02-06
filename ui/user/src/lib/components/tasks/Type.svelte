<script lang="ts">
	import { ChatService, type Task, type Version } from '$lib/services';
	import { onMount } from 'svelte';
	import Dropdown from '$lib/components/tasks/Dropdown.svelte';

	interface Props {
		task?: Task;
		onChanged?: (task: Task) => void | Promise<void>;
		editMode?: boolean;
	}

	let { task, onChanged, editMode = false }: Props = $props();

	let inputVisible = $derived.by(() => {
		if (task?.webhook || task?.schedule || task?.email) {
			return false;
		}
		return Object.keys(task?.onDemand?.params ?? {}).length === 0;
	});
	let version: Version = $state({});
	let defaultTimezone = $state(Intl.DateTimeFormat().resolvedOptions().timeZone);
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
			await onChanged?.({
				...task,
				schedule: {
					interval: 'hourly',
					hour: 0,
					minute: 0,
					day: 0,
					weekday: 0,
					timezone: defaultTimezone
				},
				webhook: undefined,
				email: undefined,
				onDemand: undefined
			});
		}
		if (value === 'webhook') {
			await onChanged?.({
				...task,
				schedule: undefined,
				webhook: {},
				email: undefined,
				onDemand: undefined
			});
		}
		if (value === 'email') {
			await onChanged?.({
				...task,
				schedule: undefined,
				webhook: undefined,
				onDemand: undefined,
				email: {}
			});
		}
		if (value === 'onDemand') {
			await onChanged?.({
				...task,
				schedule: undefined,
				webhook: undefined,
				email: undefined,
				onDemand: undefined
			});
		}
	}
</script>

<div class="flex flex-1 justify-end">
	<div class="flex items-center">
		{#if editMode}
			<button
				class="ml-2 flex items-center rounded-3xl p-2 px-4 text-gray hover:bg-gray-70 hover:text-black dark:hover:bg-gray-900 dark:hover:text-gray-50"
				class:hidden={!inputVisible}
				onclick={() => {
					if (!task) {
						return;
					}
					onChanged?.({
						...task,
						onDemand: {
							params: { '': '' }
						}
					});
				}}
			>
				Add Arguments
			</button>
		{/if}
	</div>
	{#if editMode || !inputVisible}
		<Dropdown
			disabled={!editMode}
			selected={selectedTrigger()}
			values={options}
			onSelected={selected}
		/>
	{/if}
</div>

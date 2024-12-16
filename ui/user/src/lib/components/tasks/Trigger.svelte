<script lang="ts">
	import { ChatService, type Task, type Version } from '$lib/services';
	import Schedule from '$lib/components/tasks/Schedule.svelte';
	import { onMount } from 'svelte';
	import OnDemand from '$lib/components/tasks/OnDemand.svelte';

	interface Props {
		task?: Task;
		onChanged?: (task: Task) => void | Promise<void>;
		editMode?: boolean;
	}

	let {
		task = {
			name: 'Loading...',
			steps: [],
			id: ''
		},
		onChanged,
		editMode = false
	}: Props = $props();

	let version: Version = $state({});
	let email = $derived.by(() => {
		if (version.emailDomain && task.alias) {
			return `${task.alias.replace('/', '.')}@${version.emailDomain}`;
		}
		return '';
	});
	let webhook = $derived.by(() => {
		if (typeof window !== 'undefined' && task.alias) {
			return window.location.protocol + '//' + window.location.host + '/api/webhooks/' + task.alias;
		}
		return '';
	});
	let visible: boolean = $derived.by(() => {
		if (task.webhook || task.schedule || task.email) {
			return true;
		}
		return Object.keys(task.onDemand?.params ?? {}).length > 0;
	});
	let lastParamsSeen: Record<string, string> = $state({});

	$effect(() => {
		if (task.onDemand?.params && Object.keys(task.onDemand.params).length > 0) {
			lastParamsSeen = task.onDemand.params;
		}
		if (
			Object.keys(lastParamsSeen ?? {}).length > 0 &&
			task.onDemand?.params &&
			Object.keys(task.onDemand.params).length === 0
		) {
			lastParamsSeen = task.onDemand.params;
		}
	});

	onMount(async () => {
		version = await ChatService.getVersion();
	});

	function selectedTrigger(): string {
		if (task.schedule) {
			return 'schedule';
		}
		if (task.webhook) {
			return 'webhook';
		}
		if (task.email) {
			return 'email';
		}
		return 'onDemand';
	}
</script>

{#if visible}
	<div class="mt-8 rounded-3xl bg-gray-50 p-5 dark:bg-gray-950">
		<div class="flex items-center justify-between">
			{#if selectedTrigger() === 'schedule'}
				<Schedule
					schedule={task.schedule}
					{editMode}
					onChanged={async (schedule) => {
						await onChanged?.({
							...task,
							schedule
						});
					}}
				/>
			{/if}
		</div>
		{#if selectedTrigger() === 'webhook'}
			<div class="flex justify-between pr-5">
				<h3 class="text-lg font-semibold">Webhook URL</h3>
				<div class="flex">
					{webhook}
				</div>
			</div>
		{/if}
		{#if selectedTrigger() === 'email' && email}
			<div class="flex justify-between pr-5">
				<h3 class="text-lg font-semibold">
					{#if editMode}Email{/if}
					Address
				</h3>
				{email}
			</div>
		{/if}
		{#if selectedTrigger() === 'onDemand'}
			<OnDemand
				{editMode}
				onDemand={task.onDemand ?? {}}
				onChanged={async (onDemand) => {
					await onChanged?.({
						...task,
						onDemand
					});
				}}
			/>
		{/if}
	</div>
{/if}

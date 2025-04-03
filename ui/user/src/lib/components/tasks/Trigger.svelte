<script lang="ts">
	import { type Task } from '$lib/services';
	import Schedule from '$lib/components/tasks/Schedule.svelte';
	import OnDemand from '$lib/components/tasks/OnDemand.svelte';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import { slide } from 'svelte/transition';
	import { version } from '$lib/stores';

	interface Props {
		task: Task;
		readOnly?: boolean;
	}

	let { task = $bindable(), readOnly }: Props = $props();

	let email = $derived.by(() => {
		if (version.current.emailDomain && task.alias) {
			return `${task.name ? task.name.toLocaleLowerCase().replace(/[^a-z0-9]+/g, '-') + '-' : ''}${task.alias.replace('/', '.')}@${version.current.emailDomain}`;
		}

		return '';
	});
	let webhook = $derived.by(() => {
		if (typeof window !== 'undefined' && task.alias) {
			return (
				window.location.protocol +
				'//' +
				window.location.host +
				'/api/webhooks/default/' +
				task.alias
			);
		}
		return '';
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
		if (task.onSlackMessage) {
			return 'slack';
		}
		return 'onDemand';
	}
</script>

<div transition:slide>
	<div class="flex items-center justify-between">
		{#if selectedTrigger() === 'schedule'}
			<Schedule bind:schedule={task.schedule} {readOnly} />
		{/if}
	</div>
	{#if selectedTrigger() === 'webhook'}
		<div class="flex flex-col justify-between gap-2 pr-5">
			<h4 class="text-base font-medium">Webhook URL</h4>
			<div class="bg-surface2 flex gap-2 rounded-xl px-4 py-2">
				<CopyButton text={webhook} />
				{webhook}
			</div>
		</div>
	{/if}
	{#if selectedTrigger() === 'email' && email}
		<div class="flex flex-col justify-between gap-2 pr-5">
			<h4 class="text-base font-medium">Email Address</h4>
			<div class="flex gap-2">
				<CopyButton text={email} />
				{email}
			</div>
		</div>
	{/if}
	{#if selectedTrigger() === 'onDemand'}
		<OnDemand bind:onDemand={task.onDemand} {readOnly} />
	{/if}
	{#if selectedTrigger() === 'slack'}
		<div class="flex grow flex-col gap-4">
			<div class="flex items-center justify-between">
				<div class="flex gap-2">
					<p class="text-sm text-gray-600 dark:text-gray-400">
						This task will be triggered when you mention the bot in any Slack channel
					</p>
				</div>
			</div>
		</div>
	{/if}
</div>

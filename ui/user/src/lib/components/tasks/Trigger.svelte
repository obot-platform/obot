<script lang="ts">
	import { ChatService, type Task, type Version } from '$lib/services';
	import Schedule from '$lib/components/tasks/Schedule.svelte';
	import { onMount } from 'svelte';
	import OnDemand from '$lib/components/tasks/OnDemand.svelte';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import { fade } from 'svelte/transition';

	interface Props {
		task: Task;
	}

	let { task = $bindable() }: Props = $props();

	let version: Version = $state({});
	let email = $derived.by(() => {
		if (version.emailDomain && task.alias) {
			return `${task.name ? task.name.toLocaleLowerCase().replace(/[^a-z0-9]+/g, '-') + '-' : ''}${task.alias.replace('/', '.')}@${version.emailDomain}`;
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
	<div class="rounded-2xl bg-gray-50 p-5 dark:bg-gray-950" transition:fade>
		<div class="flex items-center justify-between">
			{#if selectedTrigger() === 'schedule'}
				<Schedule bind:schedule={task.schedule} />
			{/if}
		</div>
		{#if selectedTrigger() === 'webhook'}
			<div class="flex flex-col justify-between gap-2 pr-5">
				<h3 class="text-lg font-semibold">Webhook URL</h3>
				<div class="flex gap-2 rounded-xl bg-surface2 px-4 py-2">
					<CopyButton text={webhook} />
					{webhook}
				</div>
			</div>
		{/if}
		{#if selectedTrigger() === 'email' && email}
			<div class="flex flex-col justify-between gap-2 pr-5">
				<h3 class="text-lg font-semibold">Email Address</h3>
				<div class="flex gap-2">
					<CopyButton text={email} />
					{email}
				</div>
			</div>
		{/if}
		{#if selectedTrigger() === 'onDemand'}
			<OnDemand bind:onDemand={task.onDemand} />
		{/if}
	</div>
{/if}

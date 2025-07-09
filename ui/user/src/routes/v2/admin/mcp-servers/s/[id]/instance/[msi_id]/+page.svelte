<script lang="ts">
	import BackLink from '$lib/components/admin/BackLink.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	let { mcpServer, mcpServerInstance } = data;
	const duration = PAGE_TRANSITION_DURATION;

	const mockDetails = [
		{
			id: 'kubernetes_deployments',
			label: 'Kubernetes Deployment',
			value: '-'
		},
		{
			id: 'last_restart',
			label: 'Last Restart',
			value: '-'
		},
		{
			id: 'status',
			label: 'Status',
			value: 'Healthy'
		}
	];
</script>

<Layout>
	<div class="mt-6 flex flex-col gap-6 pb-8" in:fly={{ x: 100, delay: duration, duration }}>
		{#if mcpServer}
			{@const currentLabel = mcpServerInstance?.id ?? 'Server Instance'}
			<BackLink fromURL={`/mcp-servers/${mcpServer.id}`} {currentLabel} />
		{/if}

		<h1 class="text-2xl font-semibold">
			{mcpServer?.manifest?.name} | {mcpServerInstance?.id}
		</h1>

		<div class="flex flex-col gap-2">
			{#each mockDetails as detail (detail.id)}
				<div
					class="dark:bg-surface1 dark:border-surface3 flex flex-col rounded-lg border border-transparent bg-white p-4 shadow-sm"
				>
					<div class="grid grid-cols-2 gap-4">
						<p class="text-sm font-semibold">{detail.label}</p>
						<p class="text-sm font-light">{detail.value}</p>
					</div>
				</div>
			{/each}
		</div>

		<div>
			<h2 class="mb-2 text-lg font-semibold">Events</h2>
			<div
				class="dark:bg-surface1 dark:border-surface3 flex min-h-10 flex-col rounded-lg border border-transparent bg-white p-4 shadow-sm"
			>
				<span class="text-sm font-light text-gray-400 dark:text-gray-600">No events.</span>
			</div>
		</div>

		<div>
			<h2 class="mb-2 text-lg font-semibold">Deployment Logs</h2>
			<div
				class="dark:bg-surface1 dark:border-surface3 flex min-h-64 flex-col rounded-lg border border-transparent bg-white p-4 shadow-sm"
			>
				<span class="text-sm font-light text-gray-400 dark:text-gray-600">No deployment logs.</span>
			</div>
		</div>
	</div>
</Layout>

<svelte:head>
	<title>Obot | {mcpServer?.manifest?.name} | {mcpServerInstance?.id}</title>
</svelte:head>

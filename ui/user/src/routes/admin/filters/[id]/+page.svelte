<script lang="ts">
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import FilterForm from '$lib/components/admin/FilterForm.svelte';
	import McpServerK8sInfo from '$lib/components/admin/McpServerK8sInfo.svelte';
	import AuditLogsPageContent from '$lib/components/admin/audit-logs/AuditLogsPageContent.svelte';
	import UsageGraphs from '$lib/components/admin/usage/UsageGraphs.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import type { MCPFilter } from '$lib/services/admin/types';
	import { profile } from '$lib/stores';
	import { goto } from '$lib/url';
	import { BookOpenText } from 'lucide-svelte';
	import { fly } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	let { data }: { data: { filter: MCPFilter } } = $props();
	let { filter } = $derived(data);
	let title = $derived(filter?.name ?? 'Filter');
	let selected = $derived<string>((page.url.searchParams.get('view') as string) || 'configuration');

	const tabs = [
		{ label: 'Configuration', view: 'configuration' },
		{ label: 'Server Details', view: 'server-details' },
		{ label: 'Audit Logs', view: 'audit-logs' },
		{ label: 'Usage', view: 'usage' }
	];

	function handleSelectionChange(newSelection: string) {
		if (newSelection !== selected) {
			const url = new URL(window.location.href);
			url.searchParams.set('view', newSelection);
			goto(url, { replaceState: true });
		}
	}

	const duration = PAGE_TRANSITION_DURATION;
</script>

<Layout {title} showBackButton>
	<div
		class="h-full w-full flex flex-col gap-4"
		in:fly={{ x: 100, duration }}
		out:fly={{ x: -100, duration }}
	>
		{#if filter.id}
			{@const mcpServerId = `sms1${filter.id}`}
			<div class="flex flex-1 gap-2 py-1 text-sm font-light max-h-11.5">
				{#each tabs as tab (tab.view)}
					<button
						onclick={() => {
							handleSelectionChange(tab.view);
						}}
						class={twMerge(
							'min-w-fit flex-1 rounded-md border border-transparent px-3 py-2 text-center whitespace-nowrap transition-colors duration-300',
							selected === tab.view &&
								'dark:bg-surface1 dark:border-surface3 bg-background shadow-sm',
							selected !== tab.view && 'hover:bg-surface3'
						)}
					>
						{tab.label}
					</button>
				{/each}
			</div>

			{#if selected === 'configuration'}
				<FilterForm
					{filter}
					onUpdate={() => {
						goto('/admin/filters');
					}}
					readonly={profile.current.isAdminReadonly?.()}
				/>
			{:else if selected === 'server-details'}
				<div class="flex flex-col gap-6">
					<McpServerK8sInfo
						id={filter.id}
						entity="webhook-validation"
						mcpServerId={filter.id}
						name={filter.name || ''}
						connectedUsers={[]}
						title="Details"
						classes={{
							title: 'text-lg font-semibold'
						}}
						readonly={profile.current.isAdminReadonly?.()}
					/>
				</div>
			{:else if selected === 'audit-logs'}
				<div class="mt-4 flex flex-1 flex-col gap-8 pb-8">
					<AuditLogsPageContent mcpId={mcpServerId} mcpServerDisplayName={filter.name}>
						{#snippet emptyContent()}
							<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
								<BookOpenText class="text-on-surface1 size-24 opacity-50" />
								<h4 class="text-on-surface1 text-lg font-semibold">No recent audit logs</h4>
								<p class="text-on-surface1 text-sm font-light">
									This web validation server has not had any active usage in the last 7 days.
								</p>
							</div>
						{/snippet}
					</AuditLogsPageContent>
				</div>
			{:else if selected === 'usage'}
				<div class="mt-4 flex min-h-full flex-col gap-8 pb-8">
					<UsageGraphs mcpId={mcpServerId} mcpServerDisplayName={filter.name} />
				</div>
			{/if}
		{:else}
			<FilterForm
				{filter}
				onUpdate={() => {
					goto('/admin/filters');
				}}
				readonly={profile.current.isAdminReadonly?.()}
			/>
		{/if}
	</div>
</Layout>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>

<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import FilterForm from '$lib/components/admin/FilterForm.svelte';
	import McpServerK8sInfo from '$lib/components/admin/McpServerK8sInfo.svelte';
	import AuditLogsPageContent from '$lib/components/admin/audit-logs/AuditLogsPageContent.svelte';
	import UsageGraphs from '$lib/components/admin/usage/UsageGraphs.svelte';
	import { VirtualPageViewport } from '$lib/components/ui/virtual-page';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { AdminService } from '$lib/services';
	import type { MCPFilterInput, SystemMCPServerCatalogEntry } from '$lib/services/admin/types';
	import { profile } from '$lib/stores';
	import { goto } from '$lib/url';
	import { BookOpenText, Trash2 } from 'lucide-svelte';
	import type { Component } from 'svelte';
	import { fly } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		title: string;
		filter?: MCPFilterInput;
		entry?: SystemMCPServerCatalogEntry;
		selected?: string;
		onSelectionChange?: (newSelection: string) => void;
	}

	let { title, filter, entry, onSelectionChange, selected = 'configuration' }: Props = $props();

	let deletingFilter = $state(false);

	const tabs = [
		{ label: 'Configuration', view: 'configuration' },
		{ label: 'Server Details', view: 'server-details' },
		{ label: 'Audit Logs', view: 'audit-logs' },
		{ label: 'Usage', view: 'usage' }
	];

	const duration = PAGE_TRANSITION_DURATION;
	const mcpServerId = $derived(filter?.id ? `sms1${filter.id}` : undefined);
</script>

<Layout
	main={{
		component: VirtualPageViewport as unknown as Component,
		props: {
			class: '',
			as: 'main',
			itemHeight: 56,
			overscan: 5,
			disabled: selected !== 'audit-logs'
		}
	}}
	{title}
	showBackButton
>
	<div
		class="h-full w-full flex flex-col gap-4"
		in:fly={{ x: 100, duration }}
		out:fly={{ x: -100, duration }}
	>
		{#if filter?.id}
			<div class="flex w-full items-center justify-between gap-4">
				<h1 class="flex items-center gap-4 text-2xl font-semibold">
					{title || filter.name || 'Filter'}
				</h1>
				{#if !profile.current.isAdminReadonly?.() && !entry?.id}
					<button
						class="button-destructive flex items-center gap-1 text-xs font-normal"
						use:tooltip={{ text: 'Delete Filter', placement: 'left' }}
						onclick={() => (deletingFilter = true)}
					>
						<Trash2 class="size-4" />
					</button>
				{/if}
			</div>
			<div class="flex flex-1 gap-2 py-1 text-sm font-light max-h-11.5">
				{#each tabs as tab (tab.view)}
					<button
						onclick={() => {
							onSelectionChange?.(tab.view);
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
						goto('/admin/filters', { invalidateAll: true });
					}}
					readonly={profile.current.isAdminReadonly?.()}
					mcpSystemCatalogEntryId={entry?.id || filter.systemMCPServerCatalogEntryID}
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
				onCreate={() => {
					goto('/admin/filters', { invalidateAll: true });
				}}
				readonly={profile.current.isAdminReadonly?.()}
				mcpSystemCatalogEntryId={entry?.id}
			/>
		{/if}
	</div>
</Layout>

<Confirm
	msg={`Delete ${filter?.name || 'this filter'}?`}
	show={deletingFilter}
	onsuccess={async () => {
		if (!filter?.id) return;
		await AdminService.deleteMCPFilter(filter.id);
		await goto('/admin/filters', { invalidateAll: true });
	}}
	oncancel={() => (deletingFilter = false)}
/>

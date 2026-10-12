<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import FilterForm from '$lib/components/admin/FilterForm.svelte';
	import AuditLogsPageContent from '$lib/components/admin/audit-logs/AuditLogsPageContent.svelte';
	import UsageGraphs from '$lib/components/admin/usage/UsageGraphs.svelte';
	import McpServerDetails from '$lib/components/mcp/McpServerDetails.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import { VirtualPageViewport } from '$lib/components/ui/virtual-page';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { m } from '$lib/i18n';
	import { AdminService } from '$lib/services';
	import type { MCPFilterInput, SystemMCPServerCatalogEntry } from '$lib/services/admin/types';
	import { profile } from '$lib/stores';
	import { goto } from '$lib/url';
	import { BookOpenText, Trash2 } from '@lucide/svelte';
	import type { Component } from 'svelte';
	import { fade } from 'svelte/transition';
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
		{ label: m.mcps_catalog_config_heading(), view: 'configuration' },
		{ label: m.mcps_catalog_tab_server_details(), view: 'server-details' },
		{ label: m.nav_audit_logs(), view: 'audit-logs' },
		{ label: m.nav_usage(), view: 'usage' }
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
	<div class="h-full w-full flex flex-col gap-4" in:fade={{ duration }}>
		{#if filter?.id}
			<div class="flex w-full items-center justify-between gap-4">
				<h1 class="flex items-center gap-4 text-2xl font-semibold">
					{title || filter.name || m.mcps_filters_filter()}
				</h1>
				{#if !profile.current.isAdminReadonly?.() && !entry?.id}
					<IconButton
						variant="danger2"
						tooltip={{ text: m.mcps_filters_delete_filter(), placement: 'left' }}
						onclick={() => (deletingFilter = true)}
					>
						<Trash2 class="size-4" />
					</IconButton>
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
								'dark:bg-base-200 dark:border-base-400 bg-base-100 shadow-sm',
							selected !== tab.view && 'hover:bg-base-400'
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
						goto('/mcp-servers?view=filters', { invalidateAll: true });
					}}
					readonly={profile.current.isAdminReadonly?.()}
					mcpSystemCatalogEntryId={entry?.id || filter.systemMCPServerCatalogEntryID}
				/>
			{:else if selected === 'server-details'}
				<McpServerDetails
					entity="webhook-validation"
					entityId={filter.id}
					serverId={filter.id}
					readonly={profile.current.isAdminReadonly?.()}
					connectedUsers={[]}
					k8sOverrides={{
						title: m.core_details(),
						classes: {
							title: 'text-lg font-semibold'
						}
					}}
				/>
			{:else if selected === 'audit-logs'}
				<div class="mt-4 flex flex-1 flex-col gap-8 pb-8">
					<AuditLogsPageContent mcpId={mcpServerId} mcpServerDisplayName={filter.name}>
						{#snippet emptyContent()}
							<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
								<BookOpenText class="text-muted-content size-24 opacity-50" />
								<h4 class="text-muted-content text-lg font-semibold">
									{m.mcps_filters_no_recent_audit_logs()}
								</h4>
								<p class="text-muted-content text-sm font-light">
									{m.mcps_filters_filter_no_recent_usage()}
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
					goto('/mcp-servers?view=filters', { invalidateAll: true });
				}}
				readonly={profile.current.isAdminReadonly?.()}
				mcpSystemCatalogEntryId={entry?.id}
			/>
		{/if}
	</div>
</Layout>

<Confirm
	msg={filter?.name
		? m.mcps_delete_named({ name: filter.name })
		: m.mcps_filters_delete_this_filter()}
	show={deletingFilter}
	onsuccess={async () => {
		if (!filter?.id) return;
		await AdminService.deleteMCPFilter(filter.id);
		await goto('/mcp-servers?view=filters', { invalidateAll: true });
	}}
	oncancel={() => (deletingFilter = false)}
/>

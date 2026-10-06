<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { formatFileSize } from '$lib/format';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { AdminService } from '$lib/services';
	import type { AuditLogExport } from '$lib/services/admin/types';
	import { formatTimeAgo } from '$lib/time';
	import { goto } from '$lib/url';
	import { FileArchive, CircleAlert } from '@lucide/svelte';
	import { onMount } from 'svelte';

	interface Props {
		query?: string;
		logType?: 'mcp' | 'llm';
	}

	let { query, logType = 'mcp' }: Props = $props();

	let loading = $state(false);
	let exports = $state<AuditLogExport[]>([]);
	let deleting = $state(false);
	let showDeleteConfirm = $state<
		{ type: 'single'; export: AuditLogExport } | { type: 'multi' } | undefined
	>();
	let selected = $state<Record<string, AuditLogExport>>({});

	let tableRef = $state<ReturnType<typeof Table>>();

	type ExportTableRow = AuditLogExport & {
		created: string;
		sizeDisplay: string;
	};

	let tableData = $derived.by((): ExportTableRow[] => {
		const transformedData = exports.map((exp) => ({
			...exp,
			id: exp.id || '',
			name: exp.name || '',
			state: exp.state,
			storageProvider: getProviderDisplayName(exp.storageProvider || '--'),
			error: exp.error,
			sizeDisplay: exp.exportSize ? formatFileSize(exp.exportSize) : '--',
			created: exp.createdAt
		}));

		return query
			? transformedData.filter(
					(d) =>
						d.name.toLowerCase().includes(query.toLowerCase()) ||
						d.state.toLowerCase().includes(query.toLowerCase())
				)
			: transformedData;
	});

	onMount(reload);

	// Export reload function for parent component
	export async function reload(hard = true) {
		if (!hard) {
			loading = true;
		}

		exports = await loadExports();

		loading = false;

		return exports;
	}

	async function loadExports() {
		try {
			const response = await AdminService.getAuditLogExports(logType);
			return response.items ?? [];
		} catch (error) {
			console.error('Failed to load exports:', error);
			return [];
		}
	}

	function getStatusBadgeClass(status: string): string {
		switch (status) {
			case 'completed':
				return 'badge badge-success';
			case 'processing':
				return 'badge badge-warning';
			case 'pending':
				return 'badge badge-secondary';
			case 'failed':
				return 'badge badge-destructive';
			default:
				return 'badge badge-secondary';
		}
	}

	async function handleSingleDelete(exp: AuditLogExport) {
		try {
			await AdminService.deleteAuditLogExport(exp.id);
			exports = await loadExports();
		} catch (error) {
			console.error('Failed to delete export:', error);
		}
	}

	async function handleBulkDelete() {
		for (const id of Object.keys(selected)) {
			await handleSingleDelete(selected[id]);
		}
		selected = {};
	}

	function getProviderDisplayName(provider: string): string {
		switch (provider) {
			case 's3':
				return m.audit_usage_exports_amazon_s3();
			case 'gcs':
				return m.audit_usage_exports_google_cloud_storage();
			case 'azure':
				return m.audit_usage_exports_azure_blob_storage();
			case 'custom':
				return m.audit_usage_exports_custom_s3();
		}
		return provider;
	}

	function handleRowClick(exportItem: AuditLogExport) {
		goto(
			logType === 'llm'
				? `/audit-logs/llm/exports/${exportItem.id}/view`
				: `/audit-logs/mcp/exports/${exportItem.id}/view`
		);
	}
</script>

<div class="flex flex-col gap-2">
	{#if loading}
		<div class="my-2 flex items-center justify-center">
			<Loading class="size-6" />
		</div>
	{:else if exports.length === 0}
		<div class="my-12 flex w-md flex-col items-center gap-4 self-center text-center">
			<FileArchive class="text-base-content/80 size-24 opacity-25" />
			<h4 class="text-muted-content text-lg font-semibold">{m.audit_usage_exports_none_found()}</h4>
			<p class="text-muted-content text-sm font-light">
				{m.audit_usage_exports_empty_hint()}
			</p>
		</div>
	{:else}
		<Table
			bind:this={tableRef}
			data={tableData}
			fields={['name', 'state', 'storageProvider', 'sizeDisplay', 'created']}
			filterable={['name', 'state']}
			headers={[
				{ title: m.core_name(), property: 'name' },
				{ title: m.core_status(), property: 'state' },
				{ title: m.audit_usage_exports_col_storage(), property: 'storageProvider' },
				{ title: m.core_col_size(), property: 'sizeDisplay' },
				{ title: m.core_col_created(), property: 'created' }
			]}
			sortable={['name', 'state', 'storageProvider', 'sizeDisplay', 'created']}
			noDataMessage={m.audit_usage_exports_none_found()}
			classes={{
				root: 'rounded-none rounded-b-md shadow-none'
			}}
			onClickRow={handleRowClick}
			initSort={{ property: 'created', order: 'desc' }}
		>
			{#snippet onRenderColumn(property, d)}
				{#if property === 'displayName'}
					<div class="flex shrink-0 items-center gap-2">
						<div
							class="bg-base-200 flex items-center justify-center rounded-sm p-0.5 dark:bg-base-300"
						>
							<FileArchive class="size-6" />
						</div>
						<p class="flex items-center gap-1">
							{d.name}
						</p>
					</div>
				{:else if property === 'statusDisplay'}
					<div class="flex items-center gap-1 leading-0">
						<span class={getStatusBadgeClass(d.state)}>
							{d.state}
						</span>
						{#if d.state === 'failed' && d.error}
							<button
								type="button"
								class="text-error transition-colors hover:text-error"
								use:tooltip={{
									text: d.error,
									placement: 'top',
									classes: [
										'max-w-80',
										'wrap-break-word',
										'whitespace-pre-wrap',
										'bg-base-100',
										'text-base-content',
										'border',
										'shadow-lg'
									]
								}}
							>
								<CircleAlert class="size-4" />
							</button>
						{:else if d.state === 'running'}
							<div class="size-4">
								<Loading class="size-full animate-spin duration-100" />
							</div>
						{/if}
					</div>
				{:else if property === 'created'}
					{formatTimeAgo(d.created).relativeTime}
				{:else}
					{d[property as keyof typeof d]}
				{/if}
			{/snippet}

			{#snippet tableSelectActions()}{/snippet}
		</Table>
	{/if}
</div>

<Confirm
	show={!!showDeleteConfirm}
	onsuccess={async () => {
		if (!showDeleteConfirm) return;
		deleting = true;
		if (showDeleteConfirm.type === 'single') {
			await handleSingleDelete(showDeleteConfirm.export);
		} else {
			await handleBulkDelete();
		}
		tableRef?.clearSelectAll();
		deleting = false;
		showDeleteConfirm = undefined;
	}}
	oncancel={() => (showDeleteConfirm = undefined)}
	loading={deleting}
>
	{#snippet msgContent()}
		<h4 class="flex items-center justify-center gap-2 text-lg font-semibold">
			<CircleAlert class="size-5" />
			{showDeleteConfirm?.type === 'single'
				? m.audit_usage_exports_delete_single_title()
				: m.audit_usage_exports_delete_multi_title()}
		</h4>
	{/snippet}
	{#snippet note()}
		<div class="text-sm font-light">
			{#if showDeleteConfirm?.type === 'single'}
				{m.audit_usage_exports_delete_single_note()}
			{:else}
				{m.audit_usage_exports_delete_multi_note()}
			{/if}
		</div>
	{/snippet}
</Confirm>

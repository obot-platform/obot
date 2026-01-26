<script lang="ts">
	import { AdminService } from '$lib/services';
	import Table from '$lib/components/table/Table.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import { formatTimeAgo } from '$lib/time';
	import { FileArchive, LoaderCircle, CircleAlert, AlertCircle } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import type { AuditLogExport } from '$lib/services/admin/types';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { goto } from '$lib/url';

	interface Props {
		query?: string;
	}

	let { query }: Props = $props();

	let loading = $state(false);
	let exports = $state<AuditLogExport[]>([]);
	let deleting = $state(false);
	let showDeleteConfirm = $state<
		{ type: 'single'; export: AuditLogExport } | { type: 'multi' } | undefined
	>();
	let selected = $state<Record<string, AuditLogExport>>({});

	let tableRef = $state<ReturnType<typeof Table>>();

	let tableData = $derived.by(() => {
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
			const response = await AdminService.getAuditLogExports();
			return response.items ?? [];
		} catch (error) {
			console.error('Failed to load exports:', error);
			return [];
		}
	}

	function formatFileSize(bytes: number): string {
		const units = ['B', 'KB', 'MB', 'GB'];
		let size = bytes;
		let unitIndex = 0;

		while (size >= 1024 && unitIndex < units.length - 1) {
			size /= 1024;
			unitIndex++;
		}

		return `${size.toFixed(1)} ${units[unitIndex]}`;
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
			await loadExports(); // Refresh the list
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
				return 'Amazon S3';
			case 'gcs':
				return 'Google Cloud Storage';
			case 'azure':
				return 'Azure Blob Storage';
			case 'custom':
				return 'Custom S3 Compatible';
		}
		return provider;
	}

	function handleRowClick(exportItem: AuditLogExport) {
		goto(`/admin/audit-logs/exports/${exportItem.id}/view`);
	}
</script>

<div class="flex flex-col gap-2">
	{#if loading}
		<div class="my-2 flex items-center justify-center">
			<LoaderCircle class="size-6 animate-spin" />
		</div>
	{:else if exports.length === 0}
		<div class="my-12 flex w-md flex-col items-center gap-4 self-center text-center">
			<FileArchive class="text-surface3 size-24 opacity-25" />
			<h4 class="text-on-surface1 text-lg font-semibold">No exports found.</h4>
			<p class="text-on-surface1 text-sm font-light">
				Create your first audit log export to get started.
			</p>
		</div>
	{:else}
		<Table
			bind:this={tableRef}
			data={tableData}
			fields={['name', 'state', 'storageProvider', 'sizeDisplay', 'created']}
			filterable={['name', 'state']}
			headers={[
				{ title: 'Name', property: 'name' },
				{ title: 'Status', property: 'state' },
				{ title: 'Storage', property: 'storageProvider' },
				{ title: 'Size', property: 'sizeDisplay' },
				{ title: 'Created', property: 'created' }
			]}
			sortable={['name', 'state', 'storageProvider', 'sizeDisplay', 'created']}
			noDataMessage="No exports found."
			classes={{
				root: 'rounded-none rounded-b-md shadow-none'
			}}
			onClickRow={handleRowClick}
			initSort={{ property: 'created', order: 'desc' }}
		>
			{#snippet onRenderColumn(property, d)}
				{#if property === 'displayName'}
					<div class="flex flex-shrink-0 items-center gap-2">
						<div
							class="bg-surface1 flex items-center justify-center rounded-sm p-0.5 dark:bg-gray-600"
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
								class="text-red-500 transition-colors hover:text-red-600"
								use:tooltip={{
									text: d.error,
									placement: 'top',
									classes: [
										'max-w-80',
										'break-words',
										'whitespace-pre-wrap',
										'bg-background',
										'text-gray-900',
										'border',
										'shadow-lg'
									]
								}}
							>
								<AlertCircle class="size-4" />
							</button>
						{:else if d.state === 'running'}
							<div class="size-4">
								<LoaderCircle class="size-full animate-spin duration-100" />
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
			{`Delete ${showDeleteConfirm?.type === 'single' ? 'export' : 'selected exports'}?`}
		</h4>
	{/snippet}
	{#snippet note()}
		<div class="text-sm font-light">
			{#if showDeleteConfirm?.type === 'single'}
				This export and its associated files will be permanently deleted.
			{:else}
				The selected exports and their associated files will be permanently deleted.
			{/if}
		</div>
	{/snippet}
</Confirm>

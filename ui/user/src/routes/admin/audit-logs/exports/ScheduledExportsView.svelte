<script lang="ts">
	import { AdminService } from '$lib/services';
	import Table from '$lib/components/table/Table.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import { formatTimeAgo } from '$lib/time';
	import {
		Calendar,
		LoaderCircle,
		Ellipsis,
		Trash2,
		PlayCircle,
		PauseCircle,
		CircleAlert
	} from 'lucide-svelte';
	import { onMount } from 'svelte';
	import type { ScheduledAuditLogExport } from '$lib/services/admin/types';
	import type { Schedule } from '$lib/services/chat/types';
	import type { ScheduledAuditLogExportInput } from '$lib/services/admin/types';
	import { goto } from '$lib/url';

	interface Props {
		query?: string;
		readonly?: boolean;
	}

	let { query, readonly }: Props = $props();

	let loading = $state(false);
	let scheduledExports = $state<ScheduledAuditLogExport[]>([]);
	let deleting = $state(false);
	let toggleAction = $state<{ id: string; action: 'pause' | 'resume' } | undefined>();
	let showDeleteConfirm = $state<
		| { type: 'single'; export: ScheduledAuditLogExport; onsuccess: () => void }
		| { type: 'multi' }
		| undefined
	>();
	let selected = $state<Record<string, ScheduledAuditLogExport>>({});

	let tableRef = $state<ReturnType<typeof Table>>();

	let tableData = $derived.by(() => {
		const transformedData = scheduledExports.map((exp) => ({
			...exp,
			id: exp.id || '',
			name: exp.name || '',
			enabled: exp.enabled || false,
			scheduleDisplay: getScheduleDisplayName(exp.schedule)
		}));

		return query
			? transformedData.filter((d) => d.name.toLowerCase().includes(query.toLowerCase()))
			: transformedData;
	});

	onMount(reload);

	// Export reload function for parent component
	export async function reload(hard = true) {
		if (!hard) {
			loading = true;
		}

		scheduledExports = await loadScheduledExports();

		loading = false;

		return scheduledExports;
	}

	async function loadScheduledExports() {
		try {
			const response = await AdminService.getScheduledAuditLogExports();
			return response.items ?? [];
		} catch (error) {
			console.error('Failed to load scheduled exports:', error);
			return [];
		}
	}

	function getScheduleDisplayName(schedule: Schedule): string {
		const { interval, hour, minute, weekday, day } = schedule;

		switch (interval) {
			case 'hourly':
				return `Every hour at :${minute?.toString().padStart(2, '0') || '00'}`;
			case 'daily':
				return `Daily at ${hour}:${minute?.toString().padStart(2, '0')}`;
			case 'weekly':
				return `Weekly on ${weekday} at ${hour}:${minute?.toString().padStart(2, '0')}`;
			case 'monthly':
				return `Monthly on day ${day} at ${hour}:${minute?.toString().padStart(2, '0')}`;
			default:
				return interval;
		}
	}

	async function handleUpdateScheduledExport(
		id: string,
		request: Partial<ScheduledAuditLogExportInput>
	) {
		try {
			// Set toggle action state for loading indicator
			toggleAction = { id, action: request.enabled ? 'resume' : 'pause' };

			await AdminService.updateScheduledAuditLogExport(id, request);
			await reload(); // Refresh the list
		} catch (error) {
			console.error('Failed to update scheduled export:', error);
		} finally {
			toggleAction = undefined;
		}
	}

	async function handleSingleDelete(exp: ScheduledAuditLogExport) {
		try {
			await AdminService.deleteScheduledAuditLogExport(exp.id);
			await reload(); // Refresh the list
		} catch (error) {
			console.error('Failed to delete scheduled export:', error);
		}
	}

	async function handleBulkDelete() {
		const keys = Object.keys(selected);

		await Promise.all(keys.map((id) => handleSingleDelete(selected[id])));

		if (keys.length > 0) {
			reload();
		}

		selected = {};
	}

	async function handleBulkPause() {
		const keys = Object.keys(selected);

		await Promise.all(keys.map((id) => handleUpdateScheduledExport(id, { enabled: false })));

		if (keys.length > 0) {
			reload();
		}

		selected = {};
	}

	async function handleBulkResume() {
		const keys = Object.keys(selected);

		await Promise.all(keys.map((id) => handleUpdateScheduledExport(id, { enabled: true })));

		if (keys.length > 0) {
			reload();
		}

		selected = {};
	}

	function getBulkActionState(currentSelected: Record<string, ScheduledAuditLogExport>) {
		const selectedArray = Object.values(currentSelected);
		if (selectedArray.length === 0) return null;

		const allEnabled = selectedArray.every((exp) => exp.enabled);
		const allDisabled = selectedArray.every((exp) => !exp.enabled);

		if (allEnabled) return 'pause';
		if (allDisabled) return 'resume';
		return null; // Mixed state
	}

	function handleRowClick(scheduledExport: ScheduledAuditLogExport) {
		goto(`/admin/audit-logs/exports/scheduled/${scheduledExport.id}/edit`);
	}
</script>

<div class="flex flex-col gap-2">
	{#if loading}
		<div class="my-2 flex items-center justify-center">
			<LoaderCircle class="size-6 animate-spin" />
		</div>
	{:else if scheduledExports.length === 0}
		<div class="my-12 flex w-md flex-col items-center gap-4 self-center text-center">
			<Calendar class="text-surface3 size-24 opacity-50" />
			<h4 class="text-on-surface1 text-lg font-semibold">No export schedules found.</h4>
			<p class="text-on-surface1 text-sm font-light">
				Create your first export schedule to automate your audit log exports.
			</p>
		</div>
	{:else}
		<Table
			bind:this={tableRef}
			data={tableData}
			fields={['name', 'scheduleDisplay', 'lastRunAt', 'enabled']}
			filterable={['displayName', 'scheduleDisplay']}
			headers={[
				{ title: 'Name', property: 'displayName' },
				{ title: 'Schedule', property: 'scheduleDisplay' },
				{ title: 'Last Run', property: 'lastRunAt' },
				{ title: 'Enabled', property: 'enabled' }
			]}
			sortable={['displayName', 'scheduleDisplay', 'lastRun']}
			noDataMessage="No export schedules found."
			classes={{
				root: 'rounded-none rounded-b-md shadow-none'
			}}
			onClickRow={handleRowClick}
			initSort={{ property: 'lastRunAt', order: 'desc' }}
		>
			{#snippet onRenderColumn(property, d)}
				{#if property === 'displayName'}
					<div class="flex flex-shrink-0 items-center gap-2">
						<div
							class="bg-surface1 flex items-center justify-center rounded-sm p-0.5 dark:bg-gray-600"
						>
							<Calendar class="size-6" />
						</div>
						<p class="flex items-center gap-1">
							{d.name}
						</p>
					</div>
				{:else if property === 'lastRunAt'}
					{d.lastRunAt ? formatTimeAgo(d.lastRunAt).relativeTime : '--'}
				{:else}
					{d[property as keyof typeof d]}
				{/if}
			{/snippet}
			{#snippet actions(d)}
				<DotDotDot class="icon-button hover:dark:bg-background/50">
					{#snippet icon()}
						<Ellipsis class="size-4" />
					{/snippet}

					{#snippet children({ toggle })}
						<div class="default-dialog flex min-w-max flex-col gap-1 p-2">
							{#if !readonly}
								{#if d.enabled}
									<button
										class="menu-button"
										disabled={toggleAction?.id === d.id}
										onclick={async (e) => {
											e.stopPropagation();
											await handleUpdateScheduledExport(d.id, { enabled: false });

											toggle(false);
										}}
									>
										{#if toggleAction?.id === d.id && toggleAction.action === 'pause'}
											<LoaderCircle class="size-4 animate-spin" />
										{:else}
											<PauseCircle class="size-4" />
										{/if}
										Pause Schedule
									</button>
								{:else}
									<button
										class="menu-button-primary"
										disabled={toggleAction?.id === d.id}
										onclick={async (e) => {
											e.stopPropagation();
											await handleUpdateScheduledExport(d.id, { enabled: true });
											toggle(false);
										}}
									>
										{#if toggleAction?.id === d.id && toggleAction.action === 'resume'}
											<LoaderCircle class="size-4 animate-spin" />
										{:else}
											<PlayCircle class="size-4" />
										{/if}
										Resume Schedule
									</button>
								{/if}
								<button
									class="menu-button-destructive"
									onclick={(e) => {
										e.stopPropagation();
										showDeleteConfirm = {
											type: 'single',
											export: d,
											onsuccess: () => {
												toggle(false);
											}
										};
									}}
								>
									<Trash2 class="size-4" /> Delete
								</button>
							{/if}
						</div>
					{/snippet}
				</DotDotDot>
			{/snippet}
			{#snippet tableSelectActions(currentSelected)}
				{@const bulkActionState = getBulkActionState(currentSelected)}
				<div class="flex grow items-center justify-end gap-2 px-4 py-2">
					{#if bulkActionState === 'pause'}
						<button
							class="button flex items-center gap-1 text-sm font-normal"
							onclick={() => {
								selected = currentSelected;
								handleBulkPause();
							}}
							disabled={readonly}
						>
							<PauseCircle class="size-4" /> Pause
							{#if !readonly}
								<span class="pill-primary">
									{Object.keys(currentSelected).length}
								</span>
							{/if}
						</button>
					{:else if bulkActionState === 'resume'}
						<button
							class="button flex items-center gap-1 text-sm font-normal"
							onclick={() => {
								selected = currentSelected;
								handleBulkResume();
							}}
							disabled={readonly}
						>
							<PlayCircle class="size-4" /> Resume
							{#if !readonly}
								<span class="pill-primary">
									{Object.keys(currentSelected).length}
								</span>
							{/if}
						</button>
					{/if}
					<button
						class="button flex items-center gap-1 text-sm font-normal"
						onclick={() => {
							selected = currentSelected;
							showDeleteConfirm = {
								type: 'multi'
							};
						}}
						disabled={readonly}
					>
						<Trash2 class="size-4" /> Delete
						{#if !readonly}
							<span class="pill-primary">
								{Object.keys(currentSelected).length}
							</span>
						{/if}
					</button>
				</div>
			{/snippet}
		</Table>
	{/if}
</div>

<Confirm
	msg={showDeleteConfirm?.type === 'single'
		? 'Are you sure you want to delete this scheduled export?'
		: 'Are you sure you want to delete the selected scheduled exports?'}
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
		if ('onsuccess' in showDeleteConfirm) {
			showDeleteConfirm.onsuccess();
		}
		showDeleteConfirm = undefined;
	}}
	oncancel={() => (showDeleteConfirm = undefined)}
	loading={deleting}
>
	{#snippet title()}
		<h4 class="mb-4 flex items-center justify-center gap-2 text-lg font-semibold">
			<CircleAlert class="size-5" />
			{`Delete ${showDeleteConfirm?.type === 'single' ? 'scheduled export' : 'selected scheduled exports'}?`}
		</h4>
	{/snippet}
	{#snippet note()}
		<div class="mb-8 text-sm font-light">
			{#if showDeleteConfirm?.type === 'single'}
				This scheduled export will be permanently deleted and will no longer run.
			{:else}
				The selected scheduled exports will be permanently deleted and will no longer run.
			{/if}
		</div>
	{/snippet}
</Confirm>

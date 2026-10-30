<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Search from '$lib/components/Search.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_SIZE } from '$lib/constants';
	import { m } from '$lib/i18n';
	import { AdminService, UserService, type OrgUser } from '$lib/services';
	import type { MDMDevice } from '$lib/services/admin/types';
	import { profile, version } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import { resolve } from '$lib/url';
	import { openUrl } from '$lib/utils';
	import { Laptop, Trash2 } from '@lucide/svelte';
	import { onMount } from 'svelte';

	type Row = MDMDevice & {
		hostname_display: string;
		short_device_id: string;
		os_display: string;
		user_display?: string;
		user?: OrgUser;
		mcp_count?: number;
		skill_count?: number;
		plugin_count?: number;
		client_count?: number;
		last_scanned_at?: string;
	};

	let devices = $state<MDMDevice[]>([]);
	let users = $state<OrgUser[]>([]);
	let loading = $state(true);
	let loadError = $state(false);
	let query = $state('');
	let removing = $state<Row[]>([]);
	let removeLoading = $state(false);
	let removeError = $state<string>();
	let tableRef = $state<ReturnType<typeof Table>>();

	const readOnly = $derived(Boolean(profile.current.isAdminReadonly?.()));
	const userById = $derived(new Map(users.map((user) => [user.id, user])));

	function userDisplay(user: OrgUser): string {
		return user.displayName ?? user.email ?? user.username ?? user.id;
	}

	// Fill in what the latest scan saw. Enrollment details win where both have a value,
	// since they're refreshed every time the device enrolls.
	let rows = $derived<Row[]>(
		devices.map((device) => {
			const scan = device.latestScan;
			const user = scan?.submittedBy ? userById.get(scan.submittedBy) : undefined;
			const os = [device.os, device.osVersion].filter(Boolean).join(' ') || scan?.os;
			return {
				...device,
				hostname_display: device.hostname || scan?.hostname || '—',
				short_device_id: device.deviceID.slice(0, 12),
				os_display: [os, scan?.arch].filter(Boolean).join(' / ') || '—',
				user,
				user_display: user ? userDisplay(user) : scan?.username || undefined,
				mcp_count: scan?.mcpServerCount,
				skill_count: scan?.skillCount,
				plugin_count: scan?.pluginCount,
				client_count: scan?.clientCount,
				last_scanned_at: scan?.scannedAt
			};
		})
	);

	let filteredRows = $derived.by(() => {
		const q = query.trim().toLowerCase();
		if (!q) return rows;
		return rows.filter((row) =>
			[row.deviceID, row.hostname_display, row.os_display, row.user_display].some((value) =>
				(value ?? '').toLowerCase().includes(q)
			)
		);
	});

	onMount(async () => {
		try {
			devices = await AdminService.listAllMDMDevices();
		} catch {
			loadError = true;
		} finally {
			loading = false;
		}

		// Users are only needed to name who submitted a scan, so the list doesn't wait on them
		if (devices.some((device) => device.latestScan?.submittedBy)) {
			UserService.listUsers().then((response) => {
				users = response;
			});
		}
	});

	function deviceName(row: Row): string {
		return row.hostname_display === '—' ? row.short_device_id : row.hostname_display;
	}

	// Remove the selected devices one at a time. If one fails, stop and keep the dialog open with the
	// error. The devices that were removed still drop out of the list, the selection, and the dialog,
	// so the seat count stays accurate and retrying picks up where this stopped.
	async function removeDevices() {
		const targets = removing;
		if (targets.length === 0) return;
		removeLoading = true;
		removeError = undefined;
		const removed: string[] = [];
		try {
			for (const target of targets) {
				await AdminService.deleteMDMDevice(target.deviceID, { deleteScans: true });
				removed.push(target.deviceID);
				tableRef?.clearSelect(String(target.id));
			}
			removing = [];
		} catch (err) {
			removeError = err instanceof Error ? err.message : String(err);
		} finally {
			removeLoading = false;
			if (removed.length > 0) {
				devices = devices.filter((device) => !removed.includes(device.deviceID));
				removing = removing.filter((row) => !removed.includes(row.deviceID));
				// Keep the License page's device count in step without refetching the version
				if (version.current.deviceCount !== undefined) {
					version.current.deviceCount = devices.length;
				}
			}
		}
	}
</script>

<Search
	value={query}
	class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
	onChange={(value) => (query = value)}
	placeholder={m.inventory_enforcement_devices_enrolled_search_placeholder()}
/>

{#if loading}
	<Skeleton type="table" classes={{ header: 'h-14' }} />
{:else if loadError}
	<div class="notification-error text-sm">
		{m.inventory_enforcement_devices_enrolled_load_error()}
	</div>
{:else if devices.length === 0}
	<div class="mx-auto mt-12 flex w-md flex-col items-center gap-4 text-center">
		<Laptop class="text-muted-content size-24 opacity-50" />
		<h4 class="text-muted-content text-lg font-semibold">
			{m.inventory_enforcement_devices_none_enrolled()}
		</h4>
		<p class="text-muted-content text-sm font-light">
			{m.inventory_enforcement_devices_none_enrolled_desc()}
		</p>
	</div>
{:else}
	<Table
		bind:this={tableRef}
		data={filteredRows}
		fields={[
			'hostname_display',
			'short_device_id',
			'os_display',
			'user_display',
			'mcp_count',
			'skill_count',
			'plugin_count',
			'client_count',
			'last_scanned_at',
			'enrolledAt'
		]}
		headers={[
			{ title: m.core_col_hostname(), property: 'hostname_display' },
			{ title: m.audit_usage_audit_logs_device(), property: 'short_device_id' },
			{ title: m.inventory_enforcement_devices_col_os_arch(), property: 'os_display' },
			{ title: m.core_col_user(), property: 'user_display' },
			{ title: m.inventory_enforcement_devices_col_mcp(), property: 'mcp_count' },
			{ title: m.inventory_enforcement_skills_tab(), property: 'skill_count' },
			{ title: m.inventory_enforcement_devices_col_plugins(), property: 'plugin_count' },
			{ title: m.audit_usage_exports_filter_title_clients(), property: 'client_count' },
			{ title: m.inventory_enforcement_devices_col_last_scanned(), property: 'last_scanned_at' },
			{ title: m.inventory_enforcement_devices_col_enrolled(), property: 'enrolledAt' }
		]}
		sortable={[
			'hostname_display',
			'short_device_id',
			'os_display',
			'user_display',
			'mcp_count',
			'skill_count',
			'plugin_count',
			'client_count',
			'last_scanned_at',
			'enrolledAt'
		]}
		initSort={{ property: 'enrolledAt', order: 'desc' }}
		pageSize={PAGE_SIZE}
		onClickRow={(row, isCtrlClick) => {
			openUrl(resolve(`/inventory/devices/${encodeURIComponent(row.deviceID)}`), isCtrlClick);
		}}
	>
		{#snippet onRenderColumn(property, row: Row)}
			{#if property === 'short_device_id'}
				<span title={row.deviceID}>{row.short_device_id}</span>
			{:else if property === 'user_display'}
				{#if row.user}
					<div class="flex items-center gap-2">
						<div class="size-5 shrink-0 overflow-hidden rounded-full bg-base-100 dark:bg-base-300">
							{#if row.user.iconURL}
								<img
									src={row.user.iconURL}
									class="h-full w-full object-cover"
									alt=""
									referrerpolicy="no-referrer"
								/>
							{/if}
						</div>
						<span>{row.user_display}</span>
					</div>
				{:else if row.user_display}
					<span class="font-mono text-xs">{row.user_display}</span>
				{:else}
					—
				{/if}
			{:else if property === 'last_scanned_at' || property === 'enrolledAt'}
				{@const time = formatTimeAgo(row[property])}
				{#if time.relativeTime}
					<span use:tooltip={time.fullDate}>{time.relativeTime}</span>
				{:else}
					<span class="text-muted-content">{m.inventory_enforcement_devices_never_scanned()}</span>
				{/if}
			{:else}
				{row[property as keyof Row] ?? '—'}
			{/if}
		{/snippet}
		{#snippet actions(row: Row)}
			{#if !readOnly}
				<DotDotDot>
					<button class="menu-button text-error" onclick={() => (removing = [row])}>
						<Trash2 class="size-4" />
						{m.inventory_enforcement_devices_remove()}
					</button>
				</DotDotDot>
			{/if}
		{/snippet}
		{#snippet tableSelectActions(selected)}
			{#if !readOnly}
				<div class="flex grow items-center justify-end gap-2 px-4 py-2">
					<button
						class="btn btn-secondary flex items-center gap-1 text-sm font-normal"
						onclick={() => (removing = Object.values(selected))}
					>
						<Trash2 class="size-4" />
						{m.inventory_enforcement_devices_remove()}
					</button>
				</div>
			{/if}
		{/snippet}
	</Table>
{/if}

<Confirm
	title={m.inventory_enforcement_devices_remove_title()}
	msg={removing.length === 1
		? m.inventory_enforcement_devices_remove_msg_one({ name: deviceName(removing[0]) })
		: m.inventory_enforcement_devices_remove_msg_other({ count: removing.length })}
	show={removing.length > 0}
	loading={removeLoading}
	submitText={m.inventory_enforcement_devices_remove()}
	onsuccess={removeDevices}
	oncancel={() => {
		removing = [];
		removeError = undefined;
	}}
>
	{#snippet note()}
		<p>{m.inventory_enforcement_devices_remove_note()}</p>
		{#if removeError}
			<p class="text-error mt-2">{removeError}</p>
		{/if}
	{/snippet}
</Confirm>

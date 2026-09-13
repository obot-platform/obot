<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Search from '$lib/components/Search.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import { UserService, type OrgUser, type VMCP, type VMCPInstance } from '$lib/services';
	import { vmcpInstanceAuditLogsPath, vmcpInstancePath } from '$lib/services/vmcps/utils';
	import { errors, profile, vmcpInstances } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import { formatTimeAgo } from '$lib/time';
	import { getUserDisplayName, openUrl } from '$lib/utils';
	import VMcpIcon from './VMcpIcon.svelte';
	import { Captions, Ellipsis, Layers, Trash2 } from '@lucide/svelte';

	interface Props {
		vmcps: VMCP[];
		usersMap: Map<string, OrgUser>;
	}

	let { vmcps, usersMap }: Props = $props();

	let query = $state('');
	let deleting = $state(false);
	let showDeleteConfirm = $state<DeploymentRow>();

	let vmcpsMap = $derived(new Map(vmcps.map((vmcp) => [vmcp.id, vmcp])));
	let readonly = $derived(profile.current.isAdminReadonly?.() ?? false);
	let canManage = $derived((profile.current.isAdmin?.() ?? false) && !readonly);

	type DeploymentRow = VMCPInstance & {
		displayName: string;
		userName: string;
		vmcp?: VMCP;
	};

	let tableData = $derived.by((): DeploymentRow[] => {
		const rows = vmcpInstances.current.items
			.filter((instance) => !instance.deleted)
			.map((instance) => {
				const vmcp = vmcpsMap.get(instance.vmcpID);
				return {
					...instance,
					displayName: vmcp?.displayName || instance.vmcpID,
					userName: getUserDisplayName(usersMap, instance.userID),
					vmcp
				};
			});

		const search = query.trim().toLowerCase();
		if (!search) return rows;
		return rows.filter(
			(row) =>
				row.displayName.toLowerCase().includes(search) ||
				row.userName.toLowerCase().includes(search) ||
				row.id.toLowerCase().includes(search)
		);
	});

	function canDelete(row: DeploymentRow) {
		if (readonly) return false;
		return canManage || row.userID === profile.current.id;
	}

	async function handleDelete() {
		const row = showDeleteConfirm;
		if (!row) return;
		deleting = true;
		try {
			await UserService.deleteVMCPInstance(row.id);
			vmcpInstances.remove(row.id);
			success.add(`${row.displayName} deployment deleted.`);
		} catch {
			errors.append('Failed to delete vMCP deployment.');
		} finally {
			deleting = false;
			showDeleteConfirm = undefined;
		}
	}
</script>

<div class="flex min-h-full flex-col">
	<div class="bg-base-200 dark:bg-base-100 sticky top-16 left-0 z-20 mb-2 w-full py-1">
		<Search
			class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
			value={query}
			onChange={(value) => (query = value)}
			placeholder="Search deployments..."
		/>
	</div>
	<div class="dark:bg-base-300 bg-base-100 rounded-t-md shadow-sm">
		{#if vmcpInstances.current.loading && tableData.length === 0}
			<div class="my-2 flex h-72 items-center justify-center">
				<Loading class="size-6" />
			</div>
		{:else if tableData.length > 0}
			<Table
				data={tableData}
				fields={['displayName', 'userName', 'created']}
				headers={[
					{ title: 'Name', property: 'displayName' },
					{ title: 'User', property: 'userName' }
				]}
				filterable={['displayName', 'userName']}
				sortable={['displayName', 'userName', 'created']}
				initSort={{ property: 'created', order: 'desc' }}
				noDataMessage="No deployments found."
				classes={{
					root: 'rounded-none rounded-b-md shadow-none'
				}}
				onClickRow={(d, isCtrlClick) => {
					openUrl(vmcpInstancePath(d.vmcpID, d.id), isCtrlClick);
				}}
			>
				{#snippet onRenderColumn(property, d)}
					{#if property === 'displayName'}
						<div class="flex shrink-0 items-center gap-2">
							<VMcpIcon
								class="size-6"
								components={(d.vmcp?.components ?? []).map((component) => ({
									name: component.name,
									icon: component.catalogEntry?.manifest?.icon
								}))}
							/>
							<p class="flex flex-col">{d.displayName}</p>
						</div>
					{:else if property === 'created'}
						{formatTimeAgo(d.created).relativeTime}
					{:else}
						{d[property as keyof typeof d]}
					{/if}
				{/snippet}

				{#snippet actions(d)}
					<DotDotDot
						class="hover:dark:bg-base-100/50"
						classes={{ menu: 'p-0 gap-0' }}
						ariaLabel={`Actions for ${d.displayName}`}
					>
						{#snippet icon()}
							<Ellipsis class="size-4" />
						{/snippet}

						{#snippet children({ toggle })}
							<div class="flex flex-col gap-1 p-2">
								<button
									onclick={(e) => {
										e.stopPropagation();
										openUrl(vmcpInstanceAuditLogsPath(d.vmcpID, d.userID), e.ctrlKey || e.metaKey);
										toggle(false);
									}}
									class="menu-button text-left"
								>
									<Captions class="size-4" />
									View Audit Logs
								</button>
								{#if canDelete(d)}
									<button
										class="menu-button-destructive"
										onclick={(e) => {
											e.stopPropagation();
											showDeleteConfirm = d;
											toggle(false);
										}}
									>
										<Trash2 class="size-4" /> Delete
									</button>
								{/if}
							</div>
						{/snippet}
					</DotDotDot>
				{/snippet}
			</Table>
		{:else}
			<div class="my-12 flex w-md flex-col items-center gap-4 self-center text-center">
				<Layers class="text-muted-content size-24 opacity-25" />
				<h4 class="text-muted-content text-lg font-semibold">No deployments found</h4>
				<p class="text-muted-content text-sm font-light">
					Looks like there aren't any deployments created yet. <br />
					Deployments are created as users connect to vMCPs.
				</p>
			</div>
		{/if}
	</div>
</div>

<Confirm
	show={Boolean(showDeleteConfirm)}
	onsuccess={handleDelete}
	oncancel={() => (showDeleteConfirm = undefined)}
	msg=""
	loading={deleting}
	title="Confirm Delete"
>
	{#snippet note()}
		Are you sure you want to delete the "<b>{showDeleteConfirm?.displayName ?? 'this vMCP'}</b>"
		deployment? This cannot be undone.
	{/snippet}
</Confirm>

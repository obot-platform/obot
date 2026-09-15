<script lang="ts">
	import { page } from '$app/state';
	import Confirm from '$lib/components/Confirm.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import FilterPills from '$lib/components/FilterPills.svelte';
	import Search from '$lib/components/Search.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		UserService,
		type OrgUser,
		type VMCP,
		type VMCPInstance
	} from '$lib/services';
	import {
		vmcpInstanceAuditLogsPath,
		vmcpInstanceNeedsUserConfiguration,
		vmcpInstancePath,
		vmcpHasUserAllowedConfiguration
	} from '$lib/services/vmcps/utils';
	import { errors, profile, vmcpInstances } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import { formatTimeAgo } from '$lib/time';
	import {
		clearUrlParams,
		getTableUrlParamsFilters,
		getTableUrlParamsSort,
		setFilterUrlParams,
		setSortUrlParams,
		setUrlParamAndUpdateUrl
	} from '$lib/url';
	import { getUserDisplayName, openUrl } from '$lib/utils';
	import VMcpActions from './VMcpActions.svelte';
	import VMcpIcon from './VMcpIcon.svelte';
	import { Captions, Ellipsis, Layers, ServerCog, Trash2 } from '@lucide/svelte';
	import { onMount } from 'svelte';

	interface Props {
		vmcps: VMCP[];
		usersMap: Map<string, OrgUser>;
	}

	let { vmcps, usersMap }: Props = $props();

	type VMcpDeploymentURLFilters = {
		id: string;
	};

	const query = $derived(page.url.searchParams.get('query') || '');
	const vmcpIdFilter = $derived(page.url.searchParams.get('id'));
	const pillsSearchParamFilters = $derived.by(() => {
		if (!vmcpIdFilter) return {} as Record<keyof VMcpDeploymentURLFilters, string>;
		return { id: vmcpIdFilter };
	});
	const hasFilterPills = $derived(Boolean(vmcpIdFilter));
	const initSort = $derived(getTableUrlParamsSort({ property: 'created', order: 'desc' }));
	const urlFilters = $derived.by(() => {
		const { id, ...filters } = getTableUrlParamsFilters();
		const vmcpIDs = id?.filter(Boolean) ?? [];
		if (vmcpIDs.length) {
			return { ...filters, vmcpID: vmcpIDs };
		}
		return filters;
	});

	let deleting = $state(false);
	let showDeleteConfirm = $state<DeploymentRow>();
	let vmcpActions = $state<ReturnType<typeof VMcpActions>>();

	let vmcpsMap = $derived(new Map(vmcps.map((vmcp) => [vmcp.id, vmcp])));
	let readonly = $derived(profile.current.isAdminReadonly?.() ?? false);
	let canManage = $derived((profile.current.isAdmin?.() ?? false) && !readonly);

	type DeploymentRow = VMCPInstance & {
		displayName: string;
		userName: string;
		vmcp?: VMCP;
	};

	let loading = $state(false);
	let allVMCPInstances = $state<VMCPInstance[]>([]);
	let tableData = $derived.by((): DeploymentRow[] => {
		const rows = allVMCPInstances
			.filter((instance) => !instance.deleted)
			.map((instance) => {
				const vmcp = vmcpsMap.get(instance.vmcpID);
				return {
					...instance,
					displayName: vmcp?.displayName || instance.vmcpID,
					userName: getUserDisplayName(usersMap, instance.userID),
					vmcp,
					updateStatus: vmcpInstanceNeedsUserConfiguration(instance)
						? 'Not Configured'
						: 'Configured'
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
			allVMCPInstances = allVMCPInstances.filter((instance) => instance.id !== row.id);
		} catch {
			errors.append('Failed to delete vMCP deployment.');
		} finally {
			deleting = false;
			showDeleteConfirm = undefined;
		}
	}

	function canEditInstanceConfiguration(row: DeploymentRow) {
		return Boolean(
			!readonly &&
			row.userID === profile.current.id &&
			row.vmcp &&
			vmcpInstanceNeedsUserConfiguration(row) &&
			vmcpHasUserAllowedConfiguration(row.vmcp)
		);
	}

	async function reloadInstances() {
		allVMCPInstances = await AdminService.listAllVMCPInstances();
	}

	onMount(() => {
		loading = true;
		reloadInstances().finally(() => {
			loading = false;
		});
	});

	function handleFilter(property: string, values: string[]) {
		setFilterUrlParams(property === 'vmcpID' ? 'id' : property, values);
	}

	function handleClearAllFilters() {
		clearUrlParams(Array.from(page.url.searchParams.keys()).filter((key) => key !== 'view'));
	}

	function getFilterDisplayLabel(filterKey: keyof VMcpDeploymentURLFilters) {
		return filterKey === 'id' ? 'vMCP' : filterKey;
	}

	function getFilterValue(_filterKey: keyof VMcpDeploymentURLFilters, value: string | number) {
		return vmcpsMap.get(value.toString())?.displayName ?? value.toString();
	}
</script>

<div class="flex min-h-full flex-col">
	<div class="bg-base-200 dark:bg-base-100 sticky top-16 left-0 z-20 mb-2 w-full py-1">
		<Search
			class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
			value={query}
			onChange={(value) => setUrlParamAndUpdateUrl(page.url, 'query', value)}
			placeholder="Search deployments..."
		/>
	</div>
	{#if hasFilterPills}
		<div class="mb-2">
			<FilterPills {pillsSearchParamFilters} {getFilterDisplayLabel} {getFilterValue} />
		</div>
	{/if}
	<div class="dark:bg-base-300 bg-base-100 rounded-t-md shadow-sm">
		{#if loading}
			<div class="my-2 flex h-72 items-center justify-center">
				<Loading class="size-6" />
			</div>
		{:else if tableData.length > 0}
			<Table
				data={tableData}
				fields={['displayName', 'userName', 'updateStatus', 'created']}
				headers={[
					{ title: 'Name', property: 'displayName' },
					{ title: 'User', property: 'userName' },
					{ title: 'Update Status', property: 'updateStatus' }
				]}
				filterable={['displayName', 'userName']}
				sortable={['displayName', 'userName', 'created']}
				filters={urlFilters}
				onFilter={handleFilter}
				onClearAllFilters={handleClearAllFilters}
				onSort={setSortUrlParams}
				{initSort}
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
								{#if canEditInstanceConfiguration(d) && d.vmcp}
									<button
										class="menu-button bg-warning/10 text-warning hover:bg-warning/30"
										onclick={(e) => {
											e.stopPropagation();
											vmcpActions?.openEditInstanceConfiguration(d.vmcp!, d, {
												onConnected: () => {
													void reloadInstances();
												}
											});
											toggle(false);
										}}
									>
										<ServerCog class="size-4" /> Edit Configuration
									</button>
								{/if}
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

<VMcpActions bind:this={vmcpActions} />

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

<script lang="ts">
	import { page } from '$app/state';
	import CreateEditVMcp from '$lib/components/vmcps/CreateEditVMcp.svelte';
	import VMcpList from '$lib/components/vmcps/VMcpList.svelte';
	import VMcpListSettings from '$lib/components/vmcps/VMcpListSettings.svelte';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { Group, type OrgUser, type VMCP } from '$lib/services';
	import type {
		VMcpListSettings as VMcpListSettingsType,
		VMcpSortBy
	} from '$lib/services/vmcps/types';
	import {
		buildVMcpComponentFilterOptions,
		buildVMcpCreatorFilterOptions,
		filterVMcps,
		sortVMcps,
		resolveVMcpComponents
	} from '$lib/services/vmcps/utils';
	import { mcpServersAndEntries, profile, vmcpInstances } from '$lib/stores';
	import { setFilterUrlParams, setSortUrlParams, setUrlParamAndUpdateUrl } from '$lib/url';
	import { Layers, Pencil, Plus } from '@lucide/svelte';
	import { untrack, type Snippet } from 'svelte';
	import { fade } from 'svelte/transition';

	interface Props {
		vmcps: VMCP[];
		scope: 'all' | 'mine';
		usersMap: Map<string, OrgUser>;
		onCreate?: () => void;
		onListedVmcpsChange?: (vmcps: VMCP[]) => void;
		onFilteredVmcpsChange?: (vmcps: VMCP[]) => void;
		noDataContent?: Snippet;
	}

	let {
		vmcps: sourceVMcps,
		scope,
		usersMap,
		onCreate,
		onListedVmcpsChange,
		onFilteredVmcpsChange,
		noDataContent: noDataContentProp
	}: Props = $props();

	const sortByValues: VMcpSortBy[] = ['name', 'created', 'componentServers'];

	function getInitialFilters(): VMcpListSettingsType {
		const urlSortBy = page.url.searchParams.get('sort');
		const urlDirection = page.url.searchParams.get('sortDirection');
		return {
			showMyVMcpsOnly: page.url.searchParams.get('showMyVMcpsOnly') === 'true',
			sortBy: sortByValues.includes(urlSortBy as VMcpSortBy) ? (urlSortBy as VMcpSortBy) : 'name',
			sortDirection: urlDirection === 'desc' ? 'desc' : 'asc',
			query: page.url.searchParams.get('query') || '',
			componentFilterBy: page.url.searchParams.get('components') || '',
			statusFilterBy: page.url.searchParams.get('status') || '',
			creatorFilterBy: page.url.searchParams.get('creator') || '',
			variant: (page.url.searchParams.get('variant') as 'grid' | 'table') || 'grid'
		};
	}

	let listedVMcps = $state<VMCP[]>(untrack(() => sourceVMcps));
	let isLoading = $state(false);
	let filters = $state(getInitialFilters());
	let vmcpList = $state<ReturnType<typeof VMcpList>>();

	let isAdmin = $derived(Boolean(profile.current.isAdmin?.()));
	let vmcps = $derived.by(() => {
		if (scope === 'mine') return listedVMcps;
		if (!isAdmin && filters.showMyVMcpsOnly) {
			return listedVMcps.filter((vmcp) => vmcp.creatorUserID === profile.current.id);
		}
		return listedVMcps;
	});
	let isAtLeastPoweruser = $derived(profile.current.groups.includes(Group.POWERUSER));
	let canCreate = $derived(
		scope === 'all' &&
			Boolean(onCreate) &&
			(isAtLeastPoweruser ||
				profile.current.isAdmin?.() ||
				mcpServersAndEntries.current.entries.length > 0)
	);

	function componentFilterLabel(id: string) {
		for (const vmcp of listedVMcps) {
			const component = vmcp.components?.find(
				(candidate) => candidate.mcpServerCatalogEntryID === id
			);
			if (component?.name) return component.name;
			if (component?.catalogEntry?.manifest?.name) return component.catalogEntry.manifest.name;
		}
	}

	let componentFilterOptions = $derived(
		buildVMcpComponentFilterOptions(vmcps, componentFilterLabel)
	);
	let creatorFilterOptions = $derived(buildVMcpCreatorFilterOptions(listedVMcps, usersMap));
	let createEditVMcp = $state<ReturnType<typeof CreateEditVMcp>>();
	let sortedVMcps = $derived(
		sortVMcps(
			filterVMcps(
				vmcps,
				{
					query: filters.query,
					components: filters.componentFilterBy,
					status: filters.statusFilterBy,
					creators: filters.creatorFilterBy
				},
				usersMap,
				{
					instances: vmcpInstances.current.items,
					userId: profile.current.id
				}
			),
			filters.sortBy,
			filters.query,
			usersMap,
			filters.sortDirection
		)
	);

	$effect(() => {
		listedVMcps = sourceVMcps;
	});

	$effect(() => {
		onListedVmcpsChange?.(listedVMcps);
	});

	$effect(() => {
		onFilteredVmcpsChange?.(vmcps);
	});

	function vmcpComponents(vmcp: VMCP) {
		return resolveVMcpComponents(vmcp);
	}

	function openCreate() {
		onCreate?.();
	}

	function handleTableSort(property: VMcpSortBy, direction: 'asc' | 'desc') {
		vmcpList?.resetSelection();
		filters.sortBy = property;
		filters.sortDirection = direction;
		setSortUrlParams(property, direction);
	}

	function handleChange(property: keyof VMcpListSettingsType, values: string[]) {
		vmcpList?.resetSelection();

		switch (property) {
			case 'showMyVMcpsOnly':
				if (scope === 'mine') return;
				filters.showMyVMcpsOnly = values.includes('true');
				setFilterUrlParams(property, values);
				break;
			case 'sortBy':
				filters.sortBy = sortByValues.includes(values[0] as VMcpSortBy)
					? (values[0] as VMcpSortBy)
					: 'name';
				setFilterUrlParams('sort', [filters.sortBy]);
				break;
			case 'sortDirection':
				filters.sortDirection = values[0] === 'desc' ? 'desc' : 'asc';
				setFilterUrlParams('sortDirection', [filters.sortDirection]);
				break;
			case 'query':
				filters.query = values[0] || '';
				setUrlParamAndUpdateUrl(page.url, 'query', values[0] || null);
				break;
			case 'componentFilterBy':
				filters.componentFilterBy = values.join(',');
				setFilterUrlParams('components', values);
				break;
			case 'statusFilterBy':
				filters.statusFilterBy = values.join(',');
				setFilterUrlParams('status', values);
				break;
			case 'creatorFilterBy':
				filters.creatorFilterBy = values.join(',');
				setFilterUrlParams('creator', values);
				break;
			case 'variant':
				filters.variant = values[0] as 'grid' | 'table';
				setFilterUrlParams('variant', values);
				break;
		}
	}
</script>

{#if isLoading}
	<Loading class="text-primary" />
{:else}
	<VMcpListSettings
		{filters}
		onChange={handleChange}
		{componentFilterOptions}
		{creatorFilterOptions}
	>
		{#snippet actions()}
			{#if sortedVMcps.length > 0 && filters.variant === 'grid' && !vmcpList?.isInSelectMode()}
				<div in:fade>
					<button class="btn btn-secondary" onclick={() => vmcpList?.toggleSelectMode()}>
						<Pencil class="size-4" />
						{m.vmcps_edit_mode()}
					</button>
				</div>
			{/if}
			{#if sortedVMcps.length > 0 && filters.variant === 'grid' && vmcpList?.isInSelectMode()}
				<button class="btn btn-secondary" onclick={() => vmcpList?.toggleSelectAll()}>
					{vmcpList?.isAllSelected() ? m.vmcps_deselect_all() : m.vmcps_select_all()}
				</button>
			{/if}
		{/snippet}
	</VMcpListSettings>
	<VMcpList
		bind:this={vmcpList}
		items={sortedVMcps}
		filterItems={vmcps}
		components={vmcpComponents}
		onDelete={(item) => createEditVMcp?.openDelete(item)}
		onDeleted={(deleted) => {
			listedVMcps = listedVMcps.filter((vmcp) => vmcp.id !== deleted.id);
		}}
		onUpdate={(updated) => {
			listedVMcps = listedVMcps.map((vmcp) => (vmcp.id === updated.id ? updated : vmcp));
		}}
		{usersMap}
		variant={filters.variant}
		{filters}
		onFilterChange={handleChange}
		onSort={handleTableSort}
		initSort={{
			property: filters.sortBy,
			order: filters.sortDirection
		}}
	>
		{#snippet noDataContent()}
			{#if noDataContentProp}
				{@render noDataContentProp()}
			{:else if filters.query}
				<p class="text-muted-content text-sm font-light">{m.vmcps_no_vmcps_matching_query()}</p>
			{:else}
				<div class="my-12 flex w-md flex-col items-center gap-4 self-center text-center">
					<Layers class="text-muted-content size-24 opacity-25" />
					<div>
						<h4 class="text-muted-content text-lg font-semibold">
							{profile.current.hasAdminAccess?.()
								? m.vmcps_create_a_vmcp()
								: m.vmcps_no_vmcps_available()}
						</h4>
						<p class="text-muted-content text-sm font-light">
							{profile.current.hasAdminAccess?.()
								? m.vmcps_click_below_to_get_started()
								: m.vmcps_no_vmcps_available_yet()}
						</p>
					</div>
					{#if canCreate}
						<button class="btn btn-primary" onclick={openCreate}>
							<Plus class="size-4" />
							{m.vmcps_create_vmcp_now()}
						</button>
					{/if}
				</div>
			{/if}
		{/snippet}
	</VMcpList>
{/if}

<CreateEditVMcp
	bind:this={createEditVMcp}
	onDeleted={(deleted) => {
		listedVMcps = listedVMcps.filter((vmcp) => vmcp.id !== deleted.id);
		vmcpList?.resetSelection();
	}}
/>

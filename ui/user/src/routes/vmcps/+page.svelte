<script lang="ts">
	import { page } from '$app/state';
	import TabLayout, { type TabView } from '$lib/components/TabLayout.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import ConnectAllVMcps from '$lib/components/vmcps/ConnectAllVMcps.svelte';
	import CreateEditVMcp from '$lib/components/vmcps/CreateEditVMcp.svelte';
	import VMcpDeploymentsView from '$lib/components/vmcps/VMcpDeploymentsView.svelte';
	import VMcpDesigner from '$lib/components/vmcps/VMcpDesigner.svelte';
	import VMcpList from '$lib/components/vmcps/VMcpList.svelte';
	import VMcpListSettings from '$lib/components/vmcps/VMcpListSettings.svelte';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { Group, UserService, type OrgUser, type VMCP } from '$lib/services';
	import { COMMON_AI_CLIENTS } from '$lib/services/user/constants';
	import type {
		VMcpListSettings as VMcpListSettingsType,
		VMcpSortBy
	} from '$lib/services/vmcps/types';
	import {
		buildVMcpComponentFilterOptions,
		filterVMcps,
		sortVMcps,
		resolveVMcpComponents
	} from '$lib/services/vmcps/utils';
	import { mcpServersAndEntries, profile, responsive, vmcpInstances } from '$lib/stores';
	import { goto, setFilterUrlParams, setUrlParamAndUpdateUrl } from '$lib/url';
	import { Layers, Pencil, Plus } from '@lucide/svelte';
	import { onMount, untrack } from 'svelte';
	import { fade } from 'svelte/transition';

	let { data } = $props();
	let views = $derived.by((): TabView[] =>
		profile.current.hasAdminAccess?.()
			? [
					{
						label: m.vmcps(),
						value: 'vmcps',
						content: vmcpsView,
						tooltip: m.vmcps_tab_vmcps_tooltip()
					},
					{
						label: m.vmcps_deployments(),
						value: 'deployments',
						content: deploymentsView,
						tooltip: m.vmcps_deployments_tab_tooltip()
					}
				]
			: [{ label: m.vmcps(), value: 'vmcps', content: vmcpsView }]
	);

	const options = COMMON_AI_CLIENTS.slice(0, 4);

	const sortByValues: VMcpSortBy[] = ['name', 'created', 'componentServers'];

	function getInitialFilters(): VMcpListSettingsType {
		const urlSortBy = page.url.searchParams.get('sortBy');
		return {
			showMyVMcpsOnly: page.url.searchParams.get('showMyVMcpsOnly') === 'true',
			sortBy: sortByValues.includes(urlSortBy as VMcpSortBy) ? (urlSortBy as VMcpSortBy) : 'name',
			query: page.url.searchParams.get('query') || '',
			componentFilterBy: page.url.searchParams.get('components') || '',
			statusFilterBy: page.url.searchParams.get('status') || '',
			variant: (page.url.searchParams.get('variant') as 'grid' | 'table') || 'grid'
		};
	}

	let listedVMcps = $state<VMCP[]>(untrack(() => data?.vmcps ?? []));
	let isLoading = $state(false);
	let filters = $state(getInitialFilters());
	let vmcpList = $state<ReturnType<typeof VMcpList>>();

	let vmcps = $derived.by(() => {
		if (filters.showMyVMcpsOnly) {
			return listedVMcps.filter((vmcp) => vmcp.creatorUserID === profile.current.id);
		}
		return listedVMcps;
	});
	let isAtLeastPoweruser = $derived(profile.current.groups.includes(Group.POWERUSER));
	let canCreate = $derived(
		isAtLeastPoweruser ||
			profile.current.isAdmin?.() ||
			mcpServersAndEntries.current.entries.length > 0
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

	let createEditVMcp = $state<ReturnType<typeof CreateEditVMcp>>();
	let connectAllVMcpsDialog = $state<ReturnType<typeof ConnectAllVMcps>>();

	let users = $state<OrgUser[]>([]);
	let creating = $derived(page.url.searchParams.has('new'));
	let usersMap = $derived(new Map(users.map((user) => [user.id, user])));
	let sortedVMcps = $derived(
		sortVMcps(
			filterVMcps(
				vmcps,
				{
					query: filters.query,
					components: filters.componentFilterBy,
					status: filters.statusFilterBy
				},
				usersMap,
				{
					instances: vmcpInstances.current.items,
					userId: profile.current.id
				}
			),
			filters.sortBy,
			filters.query,
			usersMap
		)
	);

	$effect(() => {
		listedVMcps = data?.vmcps ?? [];
	});

	onMount(() => {
		UserService.listUsersIncludeDeleted().then((response) => {
			users = response;
		});
	});

	function vmcpComponents(vmcp: VMCP) {
		return resolveVMcpComponents(vmcp);
	}

	function openConnectAllDialog(option: (typeof COMMON_AI_CLIENTS)[number]) {
		connectAllVMcpsDialog?.open(option);
	}

	function openCreate() {
		goto(`${page.url.pathname}?new=true`);
	}

	function hideCreate() {
		const url = new URL(page.url);
		url.searchParams.delete('new');
		goto(url, { replaceState: true });
	}

	function handleChange(property: keyof VMcpListSettingsType, values: string[]) {
		vmcpList?.resetSelection();

		switch (property) {
			case 'showMyVMcpsOnly':
				filters.showMyVMcpsOnly = values.includes('true');
				setFilterUrlParams(property, values);
				break;
			case 'sortBy':
				filters.sortBy = sortByValues.includes(values[0] as VMcpSortBy)
					? (values[0] as VMcpSortBy)
					: 'name';
				setFilterUrlParams(property, values);
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
			case 'variant':
				filters.variant = values[0] as 'grid' | 'table';
				setFilterUrlParams('variant', values);
				break;
		}
	}
</script>

{#if creating}
	<VMcpDesigner
		onBack={hideCreate}
		{usersMap}
		isFirstVMcp={!listedVMcps.some((vmcp) => vmcp.creatorUserID === profile.current.id)}
	/>
{:else}
	<TabLayout
		title={m.vmcps()}
		defaultView="vmcps"
		rightNavActions={navActions}
		{views}
		classes={{
			childrenContainer: 'max-w-full'
		}}
	/>
{/if}

{#snippet navActions(_view: string)}
	{#if !responsive.isMobile}
		<div class="flex items-center gap-2 md:mr-4">
			<p class="text-xs font-light">{m.vmcps_connect_all_vmcps_label()}</p>
			{#each options as option (option.id)}
				<IconButton
					class="btn-sm bg-base-200 hover:bg-base-400 dark:hover:bg-base-300"
					tooltip={{ text: option.alt, placement: 'bottom' }}
					onclick={() => openConnectAllDialog(option)}
				>
					<img src={option.icon} alt={option.alt} class="size-4 block dark:hidden" />
					<img
						src={option.iconDark ?? option.icon}
						alt={option.alt}
						class="size-4 hidden dark:block"
					/>
				</IconButton>
			{/each}
		</div>
	{/if}
	{#if canCreate}
		<button class="btn btn-primary" onclick={openCreate}>
			<Plus class="size-4" />
			{m.vmcps_create_vmcp()}
		</button>
	{/if}
{/snippet}

{#snippet vmcpsView()}
	{#if isLoading}
		<Loading class="text-primary" />
	{:else}
		<VMcpListSettings {filters} onChange={handleChange} {componentFilterOptions}>
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
		>
			{#snippet noDataContent()}
				{#if filters.query}
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
{/snippet}

{#snippet deploymentsView()}
	<VMcpDeploymentsView vmcps={listedVMcps} {usersMap} />
{/snippet}

<ConnectAllVMcps bind:this={connectAllVMcpsDialog} {vmcps} />

<CreateEditVMcp
	bind:this={createEditVMcp}
	onDeleted={(deleted) => {
		listedVMcps = listedVMcps.filter((vmcp) => vmcp.id !== deleted.id);
		vmcpList?.resetSelection();
	}}
/>

<svelte:head>
	<title>Obot | {creating ? m.vmcps_create_vmcp() : m.vmcps()}</title>
</svelte:head>

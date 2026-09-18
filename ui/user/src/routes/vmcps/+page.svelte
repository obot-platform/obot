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
	import Loading from '$lib/icons/Loading.svelte';
	import { Group, UserService, type OrgUser, type VMCP } from '$lib/services';
	import { COMMON_AI_CLIENTS } from '$lib/services/user/constants';
	import type { VMcpListSettingsFilters, VMcpSortBy } from '$lib/services/vmcps/types';
	import {
		buildVMcpComponentFilterOptions,
		filterVMcps,
		sortVMcps,
		resolveVMcpComponents
	} from '$lib/services/vmcps/utils';
	import { mcpServersAndEntries, profile, responsive, vmcpInstances } from '$lib/stores';
	import { goto, setFilterUrlParams, setUrlParamAndUpdateUrl } from '$lib/url';
	import { Layers, Plus } from '@lucide/svelte';
	import { onMount, untrack } from 'svelte';

	let { data } = $props();
	let views = $derived.by((): TabView[] =>
		profile.current.hasAdminAccess?.()
			? [
					{ label: 'vMCPs', value: 'vmcps', content: vmcpsView },
					{ label: 'Deployments', value: 'deployments', content: deploymentsView }
				]
			: [{ label: 'vMCPs', value: 'vmcps', content: vmcpsView }]
	);

	const options = COMMON_AI_CLIENTS.slice(0, 4);

	const sortByValues: VMcpSortBy[] = ['name', 'created', 'componentServers'];

	function getInitialFilters(): VMcpListSettingsFilters {
		const urlSortBy = page.url.searchParams.get('sortBy');
		return {
			showMyVMcpsOnly: page.url.searchParams.get('showMyVMcpsOnly') === 'true',
			sortBy: sortByValues.includes(urlSortBy as VMcpSortBy) ? (urlSortBy as VMcpSortBy) : 'name',
			query: page.url.searchParams.get('query') || '',
			componentFilterBy: page.url.searchParams.get('components') || '',
			statusFilterBy: page.url.searchParams.get('status') || ''
		};
	}

	let listedVMcps = $state<VMCP[]>(untrack(() => data?.vmcps ?? []));
	let isLoading = $state(false);
	let showMyVMcpsOnly = $state(untrack(() => getInitialFilters().showMyVMcpsOnly));
	let sortBy = $state<VMcpSortBy>(untrack(() => getInitialFilters().sortBy));
	let query = $state(untrack(() => getInitialFilters().query));
	let componentFilterBy = $state(untrack(() => getInitialFilters().componentFilterBy));
	let statusFilterBy = $state(untrack(() => getInitialFilters().statusFilterBy));
	let vmcps = $derived.by(() => {
		if (showMyVMcpsOnly) {
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
					query,
					components: componentFilterBy,
					status: statusFilterBy
				},
				usersMap,
				{
					instances: vmcpInstances.current.items,
					userId: profile.current.id
				}
			),
			sortBy,
			query,
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

	function openVMcp(vmcp: VMCP) {
		goto(`/vmcps/${vmcp.id}`);
	}

	function handleChange(property: keyof VMcpListSettingsFilters, values: string[]) {
		switch (property) {
			case 'showMyVMcpsOnly':
				showMyVMcpsOnly = values.includes('true');
				setFilterUrlParams(property, values);
				break;
			case 'sortBy':
				sortBy = sortByValues.includes(values[0] as VMcpSortBy)
					? (values[0] as VMcpSortBy)
					: 'name';
				setFilterUrlParams(property, values);
				break;
			case 'query':
				query = values[0] || '';
				setUrlParamAndUpdateUrl(page.url, 'query', values[0] || null);
				break;
			case 'componentFilterBy':
				componentFilterBy = values.join(',');
				setFilterUrlParams('components', values);
				break;
			case 'statusFilterBy':
				statusFilterBy = values.join(',');
				setFilterUrlParams('status', values);
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
		title="vMCPs"
		defaultView="vmcps"
		rightNavActions={navActions}
		{views}
		classes={{
			container: 'min-h-0',
			childrenContainer: 'max-w-full'
		}}
	/>
{/if}

{#snippet navActions(_view: string)}
	{#if !responsive.isMobile}
		<div class="flex items-center gap-2 md:mr-4">
			<p class="text-xs font-light">Connect all vMCPs:</p>
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
			<Plus class="size-4" /> Create vMCP
		</button>
	{/if}
{/snippet}

{#snippet vmcpsView()}
	{#if isLoading}
		<Loading class="text-primary" />
	{:else}
		<VMcpListSettings
			filters={{
				showMyVMcpsOnly,
				sortBy,
				query,
				componentFilterBy,
				statusFilterBy
			}}
			onChange={handleChange}
			{componentFilterOptions}
		/>
		<VMcpList
			items={sortedVMcps}
			components={vmcpComponents}
			onSelect={openVMcp}
			onDelete={(item) => createEditVMcp?.openDelete(item)}
			onUpdate={(updated) => {
				listedVMcps = listedVMcps.map((vmcp) => (vmcp.id === updated.id ? updated : vmcp));
			}}
			{usersMap}
		>
			{#snippet noDataContent()}
				<div class="my-12 flex w-md flex-col items-center gap-4 self-center text-center">
					<Layers class="text-muted-content size-24 opacity-25" />
					<div>
						<h4 class="text-muted-content text-lg font-semibold">
							{profile.current.hasAdminAccess?.() ? 'Create a vMCP!' : 'No vMCPs available'}
						</h4>
						<p class="text-muted-content text-sm font-light">
							{profile.current.hasAdminAccess?.()
								? 'Click below to get started.'
								: "Looks like there aren't any vMCPs available yet."}
						</p>
					</div>
					{#if canCreate}
						<button class="btn btn-primary" onclick={openCreate}>
							<Plus class="size-4" /> Create vMCP Now
						</button>
					{/if}
				</div>
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
	}}
/>

<svelte:head>
	<title>Obot | {creating ? 'Create vMCP' : 'vMCPs'}</title>
</svelte:head>

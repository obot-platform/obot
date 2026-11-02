<script lang="ts">
	import { page } from '$app/state';
	import { productTitle } from '$lib/branding.svelte';
	import TabLayout, { type TabView } from '$lib/components/TabLayout.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import ConnectAllVMcps from '$lib/components/vmcps/ConnectAllVMcps.svelte';
	import ConnectorsMarketplace from '$lib/components/vmcps/ConnectorsMarketplace.svelte';
	import VMcpDeploymentsView from '$lib/components/vmcps/VMcpDeploymentsView.svelte';
	import VMcpDesigner from '$lib/components/vmcps/VMcpDesigner.svelte';
	import VmcpsPageContent from '$lib/components/vmcps/VmcpsPageContent.svelte';
	import { m } from '$lib/i18n';
	import { Group, UserService, type OrgUser, type VMCP } from '$lib/services';
	import { COMMON_AI_CLIENTS } from '$lib/services/user/constants';
	import { mcpServersAndEntries, profile, responsive, vmcpInstances } from '$lib/stores';
	import { goto, setUrlParamAndUpdateUrl } from '$lib/url';
	import { Plus } from '@lucide/svelte';
	import { onMount, untrack } from 'svelte';

	let { data } = $props();
	let views = $derived.by((): TabView[] =>
		profile.current.hasAdminAccess?.()
			? [
					{
						label: m.connectors_marketplace_tab(),
						value: 'connectors',
						content: connectorsView,
						tooltip: m.connectors_marketplace_tab_tooltip()
					},
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
			: [
					{
						label: m.connectors_marketplace_tab(),
						value: 'connectors',
						content: connectorsView,
						tooltip: m.connectors_marketplace_tab_tooltip()
					},
					{ label: m.vmcps(), value: 'vmcps', content: vmcpsView }
				]
	);

	const options = COMMON_AI_CLIENTS.slice(0, 5);

	let listedVMcps = $state<VMCP[]>(untrack(() => data?.vmcps ?? []));
	let filteredVMcps = $state<VMCP[]>(untrack(() => data?.vmcps ?? []));
	let users = $state<OrgUser[]>([]);

	let isAtLeastPoweruser = $derived(profile.current.groups.includes(Group.POWERUSER));
	let canCreate = $derived(
		isAtLeastPoweruser ||
			profile.current.isAdmin?.() ||
			mcpServersAndEntries.current.entries.length > 0
	);

	let usersMap = $derived(new Map(users.map((user) => [user.id, user])));
	let connectAllVMcpsDialog = $state<ReturnType<typeof ConnectAllVMcps>>();

	let creating = $derived(page.url.searchParams.has('new'));

	$effect(() => {
		listedVMcps = data?.vmcps ?? [];
	});

	onMount(() => {
		UserService.listUsersIncludeDeleted().then((response) => {
			users = response;
		});
	});

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
</script>

{#if creating}
	<VMcpDesigner
		onBack={hideCreate}
		{usersMap}
		isFirstVMcp={!listedVMcps.some((vmcp) => vmcp.creatorUserID === profile.current.id)}
		workspaceId={data.workspaceId}
	/>
{:else}
	<TabLayout
		title={m.vmcps()}
		defaultView="connectors"
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

{#snippet connectorsView()}
	<ConnectorsMarketplace
		query={page.url.searchParams.get('connectorQuery') ?? ''}
		onQueryChange={(value) => {
			setUrlParamAndUpdateUrl(page.url, 'connectorQuery', value || null);
		}}
	/>
{/snippet}

{#snippet vmcpsView()}
	<VmcpsPageContent
		vmcps={listedVMcps}
		scope="all"
		{usersMap}
		onCreate={openCreate}
		onListedVmcpsChange={(vmcps) => {
			listedVMcps = vmcps;
		}}
		onFilteredVmcpsChange={(vmcps) => {
			filteredVMcps = vmcps;
		}}
	/>
{/snippet}

{#snippet deploymentsView()}
	<VMcpDeploymentsView vmcps={listedVMcps} {usersMap} />
{/snippet}

<ConnectAllVMcps bind:this={connectAllVMcpsDialog} vmcps={filteredVMcps} />

<svelte:head>
	<title>{productTitle(creating ? m.vmcps_create_vmcp() : m.vmcps())}</title>
</svelte:head>

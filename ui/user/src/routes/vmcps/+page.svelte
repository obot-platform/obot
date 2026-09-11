<script lang="ts">
	import { page } from '$app/state';
	import TabLayout, { type TabView } from '$lib/components/TabLayout.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import ConnectAllVMcps from '$lib/components/vmcps/ConnectAllVMcps.svelte';
	import ConnectVMcp from '$lib/components/vmcps/ConnectVMcp.svelte';
	import CreateEditVMcp from '$lib/components/vmcps/CreateEditVMcp.svelte';
	import VMcpDesigner from '$lib/components/vmcps/VMcpDesigner.svelte';
	import VMcpList from '$lib/components/vmcps/VMcpList.svelte';
	import VMcpListSettings from '$lib/components/vmcps/VMcpListSettings.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import { UserService, type OrgUser, type VMCP } from '$lib/services';
	import { COMMON_AI_CLIENTS } from '$lib/services/user/constants';
	import type { VMcpConnectOptions, VMcpSortBy } from '$lib/services/vmcps/types';
	import {
		buildVMcpComponentFilterOptions,
		filterVMcps,
		sortVMcps,
		resolveVMcpComponents
	} from '$lib/services/vmcps/utils';
	import { profile, vmcpInstances } from '$lib/stores';
	import { goto } from '$lib/url';
	import { Layers, Plus } from '@lucide/svelte';
	import { onMount, untrack } from 'svelte';

	let { data } = $props();
	let views = $derived.by((): TabView[] => [
		{ label: 'vMCPs', value: 'vmcps', content: vmcpsView }
	]);

	const options = COMMON_AI_CLIENTS.slice(0, 4);

	let listedVMcps = $state<VMCP[]>(untrack(() => data?.vmcps ?? []));
	let isLoading = $state(false);
	let showMyVMcpsOnly = $state(false);
	let sortBy = $state<VMcpSortBy>('name');
	let query = $state('');
	let componentFilterBy = $state('');
	let vmcps = $derived.by(() => {
		if (showMyVMcpsOnly) {
			return listedVMcps.filter((vmcp) => vmcp.userID === profile.current.id);
		}
		return listedVMcps;
	});
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
	let connectVMcpDialog = $state<ReturnType<typeof ConnectVMcp>>();
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
					components: componentFilterBy
				},
				usersMap
			),
			sortBy
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

	function handleConnectVMcp(vmcp: VMCP, options?: VMcpConnectOptions) {
		const vmcpInstance = vmcpInstances.current.items.find(
			(candidate) => candidate.vmcpID === vmcp.id && candidate.userID === profile.current.id
		);
		connectVMcpDialog?.open(vmcp, vmcpInstance, options);
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
</script>

{#if creating}
	<VMcpDesigner onBack={hideCreate} {usersMap} />
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
	<button class="btn btn-primary" onclick={openCreate}>
		<Plus class="size-4" /> Create vMCP
	</button>
{/snippet}

{#snippet vmcpsView()}
	{#if isLoading}
		<Loading class="text-primary" />
	{:else}
		<VMcpListSettings
			bind:showMyVMcpsOnly
			bind:sortBy
			bind:query
			bind:componentFilterBy
			{componentFilterOptions}
		/>
		<VMcpList
			items={sortedVMcps}
			components={vmcpComponents}
			onSelect={openVMcp}
			onConnect={handleConnectVMcp}
			onDelete={(item) => createEditVMcp?.openDelete(item)}
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
					{#if profile.current.hasAdminAccess?.()}
						<button class="btn btn-primary" onclick={openCreate}>
							<Plus class="size-4" /> Create vMCP Now
						</button>
					{/if}
				</div>
			{/snippet}
		</VMcpList>
	{/if}
{/snippet}

<ConnectVMcp bind:this={connectVMcpDialog} />

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

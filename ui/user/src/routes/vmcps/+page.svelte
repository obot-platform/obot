<script lang="ts">
	import { page } from '$app/state';
	import Confirm from '$lib/components/Confirm.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import TabLayout, { type TabView } from '$lib/components/TabLayout.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import ConnectAllVMcps from '$lib/components/vmcps/ConnectAllVMcps.svelte';
	import ConnectVMcp from '$lib/components/vmcps/ConnectVMcp.svelte';
	import CreateEditVMcp from '$lib/components/vmcps/CreateEditVMcp.svelte';
	import CreateEditVMcpSource from '$lib/components/vmcps/CreateEditVMcpSource.svelte';
	import VMcpDesigner from '$lib/components/vmcps/VMcpDesigner.svelte';
	import VMcpList from '$lib/components/vmcps/VMcpList.svelte';
	import VMcpListSettings from '$lib/components/vmcps/VMcpListSettings.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import { AdminService, UserService, type OrgUser, type VMCP } from '$lib/services';
	import type { GitCredential, VMcpRepository } from '$lib/services/admin/types';
	import { COMMON_AI_CLIENTS } from '$lib/services/user/constants';
	import type { VMcpSortBy } from '$lib/services/vmcps/types';
	import {
		buildVMcpComponentFilterOptions,
		filterVMcps,
		sortVMcps,
		resolveVMcpComponents
	} from '$lib/services/vmcps/utils';
	import { errors, profile, vmcpInstances } from '$lib/stores';
	import { goto } from '$lib/url';
	import SourcesView from './SourcesView.svelte';
	import { Info, Layers, Plus, TriangleAlert } from '@lucide/svelte';
	import { onDestroy, onMount, untrack } from 'svelte';
	import { SvelteMap, SvelteSet } from 'svelte/reactivity';
	import { slide } from 'svelte/transition';

	let { data } = $props();
	let isAdminReadonly = $derived(Boolean(profile.current.isAdminReadonly?.()));
	let views = $derived.by((): TabView[] => [
		{ label: 'vMCPs', value: 'vmcps', content: vmcpsView },
		{ label: 'Sources', value: 'sources', content: sourcesView }
	]);

	const options = COMMON_AI_CLIENTS.slice(0, 4);

	let listedVMcps = $state<VMCP[]>(untrack(() => data?.vmcps ?? []));
	let isLoading = $state(false);
	let showMyVMcpsOnly = $state(false);
	let sortBy = $state<VMcpSortBy>('name');
	let nameFilterBy = $state('');
	let ownerFilterBy = $state('');
	let componentFilterBy = $state('');
	let vmcps = $derived(
		showMyVMcpsOnly ? listedVMcps.filter((vmcp) => vmcp.userID === profile.current.id) : listedVMcps
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
	let connectVMcpDialog = $state<ReturnType<typeof ConnectVMcp>>();
	let connectAllVMcpsDialog = $state<ReturnType<typeof ConnectAllVMcps>>();
	let createEditVMcpSource = $state<ReturnType<typeof CreateEditVMcpSource>>();

	let users = $state<OrgUser[]>([]);
	let creating = $derived(page.url.searchParams.has('new'));
	let usersMap = $derived(new Map(users.map((user) => [user.id, user])));
	let sortedVMcps = $derived(
		sortVMcps(
			filterVMcps(
				vmcps,
				{
					names: nameFilterBy,
					owners: ownerFilterBy,
					components: componentFilterBy
				},
				usersMap
			),
			sortBy
		)
	);

	let syncing = new SvelteSet<string>();
	let isSyncing = $derived(syncing.size > 0);
	let deleting = $state(false);
	let deletingSources = $state<VMcpRepository[] | undefined>();
	let vmcpRepositories = $state<VMcpRepository[]>(untrack(() => data?.vmcpRepositories ?? []));
	let gitCredentials = $state<GitCredential[]>(untrack(() => data?.gitCredentials ?? []));
	let syncErrorDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let syncError = $state<{ url: string; error: string }>();
	let syncInterval = new SvelteMap<string, ReturnType<typeof setInterval>>();

	$effect(() => {
		listedVMcps = data?.vmcps ?? [];
	});

	$effect(() => {
		vmcpRepositories = data?.vmcpRepositories ?? [];
	});

	$effect(() => {
		gitCredentials = data?.gitCredentials ?? [];
	});

	onMount(() => {
		UserService.listUsersIncludeDeleted().then((response) => {
			users = response;
		});
	});

	function clearSyncInterval(id: string) {
		if (syncInterval.get(id)) {
			clearInterval(syncInterval.get(id));
			syncInterval.delete(id);
		}
	}

	function pollTillSyncComplete(id: string) {
		if (syncInterval.get(id)) {
			clearInterval(syncInterval.get(id));
		}

		syncInterval.set(
			id,
			setInterval(async () => {
				try {
					const response = await AdminService.getVMcpRepository(id);
					if (response && !response.isSyncing) {
						clearSyncInterval(id);
						vmcpRepositories = await AdminService.listVMcpRepositories();
						syncing.delete(id);
					}
				} catch (err) {
					errors.append(`Failed to sync vMCP repository: ${err}`);
					clearSyncInterval(id);
					syncing.delete(id);
				}
			}, 5000)
		);
	}

	async function sync(id: string) {
		syncing.add(id);
		try {
			await AdminService.refreshVMcpRepository(id);
			pollTillSyncComplete(id);
		} catch (err) {
			errors.append(`Failed to refresh vMCP repository sync status: ${err}`);
			syncing.delete(id);
		}
	}

	function openVMcpsForRepository() {
		goto(`${page.url.pathname}?view=vmcps`);
	}

	function vmcpComponents(vmcp: VMCP) {
		return resolveVMcpComponents(vmcp);
	}

	function handleConnectVMcp(vmcp: VMCP) {
		const vmcpInstance = vmcpInstances.current.items.find(
			(candidate) => candidate.vmcpID === vmcp.id && candidate.userID === profile.current.id
		);
		connectVMcpDialog?.open(vmcp, vmcpInstance);
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

	onDestroy(() => {
		for (const interval of syncInterval.values()) {
			clearInterval(interval);
		}
	});
</script>

{#if creating}
	<VMcpDesigner onBack={hideCreate} />
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

{#snippet navActions(view: string)}
	{#if view === 'sources'}
		{#if !isAdminReadonly}
			<button
				class="btn btn-primary flex items-center gap-1 text-sm"
				onclick={() => createEditVMcpSource?.openAdd()}
			>
				<Plus class="size-4" /> Add Source URL
			</button>
		{/if}
	{:else}
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
	{/if}
{/snippet}

{#snippet vmcpsView()}
	{#if isLoading}
		<Loading class="text-primary" />
	{:else}
		<VMcpListSettings
			bind:showMyVMcpsOnly
			bind:sortBy
			bind:ownerFilterBy
			bind:componentFilterBy
			{componentFilterOptions}
		/>
		<VMcpList
			items={sortedVMcps}
			components={vmcpComponents}
			onSelect={openVMcp}
			onConnect={handleConnectVMcp}
			onDelete={(item) => createEditVMcp?.openDelete(item)}
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

{#snippet sourcesView()}
	{#if isSyncing}
		<div class="p-4" transition:slide={{ axis: 'y' }}>
			<div class="notification-info p-3 text-sm font-light">
				<div class="flex items-center gap-3">
					<Info class="size-6" />
					<div>The system is currently syncing with your configured Git repositories.</div>
				</div>
			</div>
		</div>
	{/if}
	<SourcesView
		{vmcpRepositories}
		syncingIds={syncing}
		{isAdminReadonly}
		onEdit={(repository) => createEditVMcpSource?.openEdit(repository)}
		onDelete={(repositories) => (deletingSources = repositories)}
		onSync={sync}
		onOpenSyncError={(url, error) => {
			syncError = { url, error };
			syncErrorDialog?.open();
		}}
		onSelectRepository={openVMcpsForRepository}
	/>
{/snippet}

<ConnectVMcp bind:this={connectVMcpDialog} />

<ConnectAllVMcps bind:this={connectAllVMcpsDialog} {vmcps} />

<CreateEditVMcp
	bind:this={createEditVMcp}
	onDeleted={(deleted) => {
		listedVMcps = listedVMcps.filter((vmcp) => vmcp.id !== deleted.id);
	}}
/>

<CreateEditVMcpSource
	bind:this={createEditVMcpSource}
	{gitCredentials}
	{vmcpRepositories}
	onSaved={(response) => {
		vmcpRepositories = vmcpRepositories.some((repository) => repository.id === response.id)
			? vmcpRepositories.map((repository) =>
					repository.id === response.id ? response : repository
				)
			: [...vmcpRepositories, response];
		sync(response.id);
	}}
/>

<Confirm
	msg={deletingSources
		? deletingSources.length === 1
			? `Delete ${deletingSources[0].displayName}?`
			: `Delete the following Git Source URLs?`
		: 'Confirm Delete'}
	show={Boolean(deletingSources && deletingSources.length > 0)}
	onsuccess={async () => {
		if (!deletingSources) return;
		deleting = true;
		try {
			for (const source of deletingSources) {
				await AdminService.deleteVMcpRepository(source.id);
			}
			vmcpRepositories = await AdminService.listVMcpRepositories();
		} catch (error) {
			errors.append(`Failed to delete Git Source URLs: ${error}`);
		} finally {
			deletingSources = undefined;
			deleting = false;
		}
	}}
	oncancel={() => (deletingSources = undefined)}
	loading={deleting}
>
	{#snippet note()}
		{#if deletingSources && deletingSources.length > 1}
			<ul class="mb-3">
				{#each deletingSources as source (source.id)}
					<li>{source.displayName}</li>
				{/each}
			</ul>
		{/if}
		<p>
			Are you sure you want to delete {deletingSources && deletingSources.length > 1
				? 'these'
				: 'this'}? This will delete all related vMCPs and their information from the system.
		</p>
	{/snippet}
</Confirm>

<ResponsiveDialog title="Git Source URL Sync" bind:this={syncErrorDialog} class="md:w-2xl">
	<div class="mb-4 flex flex-col gap-4">
		<div class="notification-alert flex flex-col gap-2">
			<div class="flex items-center gap-2">
				<TriangleAlert class="size-6 shrink-0 self-start text-warning" />
				<p class="my-0.5 flex flex-col text-sm font-semibold">
					An issue occurred fetching this source URL:
				</p>
			</div>
			<span class="text-sm font-light break-all">{syncError?.error}</span>
		</div>
	</div>
</ResponsiveDialog>

<svelte:head>
	<title>Obot | {creating ? 'Create vMCP' : 'vMCPs'}</title>
</svelte:head>

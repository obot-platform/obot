<script lang="ts">
	import { page } from '$app/state';
	import Confirm from '$lib/components/Confirm.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { parseErrorContent } from '$lib/errors';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		type MDMAsset,
		type MDMAssetSource,
		type MDMConfiguration,
		type MDMConfigurationCreateResponse
	} from '$lib/services';
	import { profile } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import { goto, getTableUrlParamsSort, setSortUrlParams } from '$lib/url';
	import { openUrl } from '$lib/utils';
	import CreateMDMConfigurationWizard from './CreateMDMConfigurationWizard.svelte';
	import { MonitorSmartphone, Plus, RefreshCw, Trash2, TriangleAlert } from '@lucide/svelte';
	import { onDestroy, onMount, untrack } from 'svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	let configurations = $state<MDMConfiguration[]>(untrack(() => data.configurations));
	let assetSource = $state<MDMAssetSource | undefined>(untrack(() => data.assetSource));
	let assets = $state<MDMAsset[]>(untrack(() => data.assets));
	let assetLoadError = $state<string | undefined>(untrack(() => data.assetLoadError));
	let refreshTimer: ReturnType<typeof setInterval> | undefined;

	let deletingConfiguration = $state<MDMConfiguration>();
	let loading = $state(false);
	let showCreateNew = $derived(page.url.searchParams.has('new'));
	let initSort = $derived(getTableUrlParamsSort({ property: 'createdAt', order: 'desc' }));

	const tableData = $derived(
		configurations.map((d) => ({
			...d,
			createdAtDisplay: formatTimeAgo(d.createdAt).relativeTime
		}))
	);
	let latestAsset = $derived(assets.find((asset) => asset.digest === assetSource?.latestDigest));
	let catalogHasTargets = $derived((latestAsset?.configurations.length ?? 0) > 0);

	function stopRefreshPolling() {
		if (refreshTimer) clearInterval(refreshTimer);
		refreshTimer = undefined;
	}

	function startRefreshPolling() {
		stopRefreshPolling();
		refreshTimer = setInterval(async () => {
			try {
				assetSource = await AdminService.getMDMAssetSource();
				if (!assetSource.isSyncing) {
					stopRefreshPolling();
					assets = await AdminService.listMDMAssets();
				}
			} catch (error) {
				stopRefreshPolling();
				assetLoadError = parseErrorContent(error).message;
			}
		}, 5000);
	}

	async function handleRefreshAssets() {
		assetLoadError = undefined;
		if (assetSource) assetSource = { ...assetSource, isSyncing: true };
		try {
			await AdminService.refreshMDMAssetSource();
			assetSource = await AdminService.getMDMAssetSource();
			if (assetSource.isSyncing) {
				startRefreshPolling();
			} else {
				assets = await AdminService.listMDMAssets();
			}
		} catch (error) {
			if (assetSource) assetSource = { ...assetSource, isSyncing: false };
			assetLoadError = parseErrorContent(error).message;
		}
	}

	onMount(() => {
		if (assetSource?.isSyncing) startRefreshPolling();
	});

	onDestroy(stopRefreshPolling);

	async function handleDelete() {
		const configurationToDelete = deletingConfiguration;
		if (!configurationToDelete) return;
		loading = true;
		try {
			await AdminService.deleteMDMConfiguration(configurationToDelete.id);
			configurations = configurations.filter((d) => d.id !== configurationToDelete.id);
		} finally {
			loading = false;
			deletingConfiguration = undefined;
		}
	}

	function handleCreate(result: MDMConfigurationCreateResponse) {
		configurations = [result, ...configurations];
	}

	function showCreateForm() {
		const url = new URL(page.url);
		url.searchParams.set('new', 'true');
		goto(url);
	}

	function hideCreateForm() {
		const url = new URL(page.url);
		url.searchParams.delete('new');
		goto(url, { replaceState: true });
	}

	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());
	const duration = PAGE_TRANSITION_DURATION;
</script>

<Layout
	title={showCreateNew ? 'New MDM Configuration' : 'MDM Configurations'}
	showBackButton={showCreateNew}
>
	{#if showCreateNew}
		<div
			class="h-full w-full"
			in:fly={{ x: 100, delay: duration, duration }}
			out:fly={{ x: -100, duration }}
		>
			<CreateMDMConfigurationWizard
				{assetSource}
				{assets}
				onCreate={handleCreate}
				onCancel={hideCreateForm}
			/>
		</div>
	{:else}
		<div class="flex flex-col gap-4">
			<div class="paper flex flex-col gap-3 p-4">
				<div class="flex flex-wrap items-start justify-between gap-3">
					<div class="flex flex-col gap-1">
						<div class="flex flex-wrap items-center gap-2">
							<span class="text-sm font-medium">MDM Asset Source</span>
							{#if assetSource?.isSyncing}
								<span class="text-muted-content flex items-center gap-1 text-xs">
									<Loading class="size-3.5" /> Refreshing
								</span>
							{:else if latestAsset}
								<span class="text-muted-content text-xs"
									>ObotSentry {latestAsset.obotSentryVersion}</span
								>
							{:else}
								<span class="text-muted-content text-xs">No stored bundle</span>
							{/if}
						</div>
						<span class="text-muted-content text-xs break-all">
							{assetSource?.source || 'Not configured at server startup'}
						</span>
						{#if assetSource?.lastSyncTime}
							<span class="text-muted-content text-xs">
								Last refreshed {formatTimeAgo(assetSource.lastSyncTime).relativeTime}
							</span>
						{/if}
					</div>
					{#if !isAdminReadonly}
						<button
							class="btn btn-secondary btn-sm flex items-center gap-1"
							disabled={!assetSource?.source || assetSource.isSyncing}
							onclick={handleRefreshAssets}
						>
							<RefreshCw class="size-3.5" /> Refresh
						</button>
					{/if}
				</div>
				<p class="text-muted-content text-xs">
					The source is set at server startup and cannot be changed here. Configurations keep using
					the bundle they were saved with.
				</p>

				{#if assetSource?.syncError || assetLoadError}
					<div class="notification-alert flex items-start gap-2 p-3">
						<TriangleAlert class="size-5 shrink-0 text-warning" />
						<div class="flex flex-col gap-1">
							<span class="text-sm font-medium">
								{catalogHasTargets
									? 'The latest asset refresh failed'
									: 'MDM assets are unavailable'}
							</span>
							<span class="text-muted-content text-xs break-all">
								{assetSource?.syncError ?? assetLoadError}
							</span>
						</div>
					</div>
				{/if}
			</div>

			{#if configurations.length === 0}
				<div class="mt-26 flex w-md flex-col items-center gap-4 self-center text-center">
					<MonitorSmartphone class="text-muted-content size-24 opacity-50" />
					<h4 class="text-muted-content text-lg font-semibold">No MDM configurations</h4>
					<p class="text-muted-content text-sm font-light">
						An MDM configuration is a fleet your managed devices enroll into via Microsoft Intune or
						Jamf Pro. <br />
						Click "New Configuration" above to generate everything you need.
					</p>
				</div>
			{:else}
				<p class="text-muted text-sm">
					Fleets that managed devices enroll into. Each configuration has enrollment keys you
					distribute through your MDM.
				</p>
				<Table
					data={tableData}
					fields={['name', 'description', 'createdAt']}
					headers={[
						{ title: 'Name', property: 'name' },
						{ title: 'Description', property: 'description' },
						{ title: 'Created', property: 'createdAt' }
					]}
					filterable={['name']}
					sortable={['name', 'createdAt']}
					{initSort}
					onSort={setSortUrlParams}
					onClickRow={(d, isCtrlClick) => {
						openUrl(`/admin/mdm-configurations/${d.id}`, isCtrlClick);
					}}
				>
					{#snippet onRenderColumn(property, d)}
						{#if property === 'description'}
							<span class="text-muted">{d.description || '-'}</span>
						{:else if property === 'createdAt'}
							{d.createdAtDisplay}
						{:else}
							{d[property as keyof typeof d]}
						{/if}
					{/snippet}
					{#snippet actions(d)}
						{#if !isAdminReadonly}
							<DotDotDot>
								<button class="menu-button text-error" onclick={() => (deletingConfiguration = d)}>
									<Trash2 class="size-4" />
									Delete
								</button>
							</DotDotDot>
						{/if}
					{/snippet}
				</Table>
			{/if}
		</div>
	{/if}

	{#snippet rightNavActions()}
		{#if !showCreateNew && !isAdminReadonly}
			<button class="btn btn-primary flex items-center gap-2 text-sm" onclick={showCreateForm}>
				<Plus class="size-4" />
				New Configuration
			</button>
		{/if}
	{/snippet}
</Layout>

<Confirm
	msg={`Delete MDM configuration "${deletingConfiguration?.name}"? Its enrollment keys stop working, but already-enrolled devices are preserved.`}
	show={Boolean(deletingConfiguration)}
	{loading}
	onsuccess={handleDelete}
	oncancel={() => (deletingConfiguration = undefined)}
/>

<svelte:head>
	<title>Obot | MDM Configurations</title>
</svelte:head>

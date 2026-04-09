<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Search from '$lib/components/Search.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { AdminService } from '$lib/services';
	import type { SkillRepository } from '$lib/services/admin/types';
	import type { Skill } from '$lib/services/nanobot/types';
	import { errors, profile } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import { goto } from '$lib/url.js';
	import { openUrl } from '$lib/utils.js';
	import {
		TriangleAlert,
		Info,
		LoaderCircle,
		PencilRuler,
		Plus,
		RefreshCcw,
		Trash2,
		X
	} from 'lucide-svelte';
	import { onDestroy, untrack } from 'svelte';
	import { SvelteSet, SvelteMap } from 'svelte/reactivity';
	import { slide } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	let { data } = $props();
	let query = $derived(page.url.searchParams.get('query') ?? '');
	let view = $derived<'skills' | 'urls'>(
		((page.url.searchParams.get('view') as 'skills' | 'urls') ?? 'skills') === 'urls'
			? 'urls'
			: 'skills'
	);
	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());
	let syncing = new SvelteSet<string>();
	let isSyncing = $derived(syncing.size > 0);

	let deleting = $state(false);
	let deletingSources = $state<SkillRepository[] | undefined>();

	let skills = $state<Skill[]>(untrack(() => data?.skills ?? []));
	let skillRepositories = $state<SkillRepository[]>(untrack(() => data.skillRepositories));
	let skillRepositoriesTableData = $derived(
		(query
			? skillRepositories.filter(
					(d) =>
						d.displayName.toLowerCase().includes(query.toLowerCase()) ||
						d.repoURL.toLowerCase().includes(query.toLowerCase())
				)
			: skillRepositories
		).map((d) => ({
			...d,
			isSyncing: syncing.has(d.id)
		}))
	);
	let skillRepositoriesMap = $derived(new Map(skillRepositories.map((d) => [d.id, d])));
	let skillsTableData = $derived(
		(query
			? skills.filter(
					(d) =>
						d.name?.toLowerCase().includes(query.toLowerCase()) ||
						d.description?.toLowerCase().includes(query.toLowerCase())
				)
			: skills
		).map((d) => ({
			...d,
			repository: d.repoID ? (skillRepositoriesMap.get(d.repoID)?.displayName ?? '') : ''
		}))
	);

	let sourceDialog = $state<HTMLDialogElement | undefined>(undefined);
	let syncErrorDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let syncError = $state<{ url: string; error: string }>();
	let syncInterval = new SvelteMap<string, ReturnType<typeof setInterval>>();

	let editingSource = $state<
		{ index: number; value: string; name: string; ref: string } | undefined
	>(undefined);
	let sourceError = $state<string | undefined>(undefined);
	let saving = $state(false);

	function switchView(newView: 'skills' | 'urls') {
		goto(resolve(`/admin/skills?view=${newView}`), { replaceState: true });
	}

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
					const response = await AdminService.getSkillRepository(id);
					if (response && !response.isSyncing) {
						clearSyncInterval(id);
						skillRepositories = await AdminService.listSkillRepositories();
						skills = await AdminService.listAllSkills();
						syncing.delete(id);
					}
				} catch (err) {
					errors.append(`Failed to sync skill repository: ${err}`);
					clearSyncInterval(id);
					syncing.delete(id);
				}
			}, 5000)
		);
	}

	onDestroy(() => {
		for (const interval of syncInterval.values()) {
			clearInterval(interval);
		}
	});

	async function sync(id: string) {
		syncing.add(id);
		try {
			await AdminService.refreshSkillRepository(id);
			pollTillSyncComplete(id);
		} catch (err) {
			errors.append(`Failed to refresh skill repository sync status: ${err}`);
			syncing.delete(id);
		}
	}

	function updateSearchQuery(value: string) {
		const params = new URLSearchParams({ view, query: value });
		goto(resolve(`/admin/skills?${params.toString()}`), {
			replaceState: true,
			keepFocus: true
		});
	}

	function closeSourceDialog() {
		editingSource = undefined;
		sourceError = undefined;
		saving = false;
		sourceDialog?.close();
	}
</script>

<Layout classes={{ navbar: 'bg-surface1' }} title="Skills">
	<div class="flex min-h-full flex-col gap-8">
		<div class="flex min-h-full flex-col">
			<div class="bg-surface1 dark:bg-background sticky top-16 left-0 z-20 w-full py-1">
				<div class="mb-2">
					<Search
						class="dark:bg-surface1 dark:border-surface3 bg-background border border-transparent shadow-sm"
						value={query}
						onChange={updateSearchQuery}
						placeholder={view == 'skills' ? 'Search skills...' : 'Search sources...'}
					/>
				</div>
			</div>
			<div class="dark:bg-surface2 bg-background rounded-t-md shadow-sm">
				<div class="flex">
					<button
						class={twMerge('page-tab max-w-1/2', view === 'skills' && 'page-tab-active')}
						onclick={() => switchView('skills')}
					>
						Skills
					</button>
					<button
						class={twMerge('page-tab max-w-1/2', view === 'urls' && 'page-tab-active')}
						onclick={() => switchView('urls')}
					>
						Sources
					</button>
				</div>

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

				{#if view === 'skills'}
					{@render skillsView()}
				{:else if view === 'urls'}
					{@render sourceUrlsView()}
				{/if}
			</div>
		</div>
	</div>
	{#snippet rightNavActions()}
		{#if !isAdminReadonly}
			<button
				class="button-primary flex items-center gap-1 text-sm"
				onclick={() => {
					editingSource = { index: -1, value: '', name: '', ref: 'main' };
					sourceDialog?.showModal();
				}}
			>
				<Plus class="size-4" /> Add Source URL
			</button>
		{/if}
	{/snippet}
</Layout>

{#snippet skillsView()}
	<div class="flex flex-col gap-2">
		{#if skills.length > 0}
			<Table
				data={skillsTableData}
				fields={['displayName', 'description', 'created', 'repository']}
				noDataMessage="No skills found."
				classes={{
					root: 'rounded-none rounded-b-md shadow-none'
				}}
				columnMaxWidths={{ created: 240 }}
				sortable={['displayName', 'created', 'repository']}
				filterable={['repository']}
				headers={[
					{
						title: 'Name',
						property: 'displayName'
					}
				]}
				onClickRow={(d, isCtrlClick) => {
					if (d.valid) {
						const url = `/admin/skills/${d.id}`;
						openUrl(url, isCtrlClick);
					}
				}}
				setRowClasses={(d) => {
					if (d.validationError) {
						return 'opacity-50 cursor-default dark:hover:bg-transparent hover:bg-transparent';
					}
					return '';
				}}
			>
				{#snippet onRenderColumn(property, d)}
					{#if property === 'displayName'}
						<span class="flex items-center gap-2">
							{d.displayName}
							{#if d.validationError}
								<div use:tooltip={{ text: d.validationError }}>
									<TriangleAlert class="size-3 text-yellow-500" />
								</div>
							{/if}
						</span>
					{:else if property === 'created'}
						{formatTimeAgo(d.created).relativeTime}
					{:else if property === 'repository'}
						<span class="block min-w-0 truncate">{d.repository}</span>
					{:else}
						{d[property as keyof typeof d]}
					{/if}
				{/snippet}
				{#snippet actions(_d)}
					<div></div>
				{/snippet}
			</Table>
		{:else}
			<div class="my-12 flex w-md flex-col items-center gap-4 self-center text-center">
				<PencilRuler class="text-surface3 size-24" />
				<h4 class="text-on-surface1 text-lg font-semibold">No current skills.</h4>
				<p class="text-on-surface1 text-sm font-light">
					Once a Git Source URL has been added, the skills <br />
					discovered will be viewable from here.
				</p>
			</div>
		{/if}
	</div>
{/snippet}

{#snippet sourceUrlsView()}
	<div class="flex flex-col gap-2">
		{#if skillRepositories.length > 0}
			<Table
				data={skillRepositoriesTableData}
				fields={['displayName', 'repoURL']}
				headers={[
					{
						property: 'displayName',
						title: 'Name'
					},
					{
						property: 'repoURL',
						title: 'URL'
					}
				]}
				noDataMessage="No Git Source URLs added."
				setRowClasses={(d) => {
					if (d.syncError) {
						return 'bg-yellow-500/10';
					}
					return '';
				}}
				classes={{
					root: 'rounded-none rounded-b-md shadow-none'
				}}
				sortable={['displayName']}
			>
				{#snippet actions(d)}
					{#if !isAdminReadonly}
						<button
							class="icon-button hover:text-red-500"
							onclick={(e) => {
								e.stopPropagation();
								deletingSources = [d];
							}}
						>
							<Trash2 class="size-4" />
						</button>
					{/if}
				{/snippet}
				{#snippet onRenderColumn(property, d)}
					{#if property === 'repoURL'}
						<div class="flex items-center gap-2">
							<p>{d.repoURL}</p>
							{#if d.syncError}
								<button
									onclick={(e) => {
										e.stopPropagation();
										syncError = {
											url: d.repoURL,
											error: d.syncError ?? ''
										};
										syncErrorDialog?.open();
									}}
									use:tooltip={{
										text: 'An issue occurred. Click to see more details.',
										classes: ['break-words']
									}}
								>
									<TriangleAlert class="size-4 text-yellow-500" />
								</button>
							{/if}
						</div>
					{:else}
						{d[property as keyof typeof d]}
					{/if}
				{/snippet}
				{#snippet tableSelectActions(currentSelected)}
					<div class="flex grow items-center justify-end gap-2 px-4 py-2">
						<button
							class="button flex items-center gap-1 text-sm font-normal"
							onclick={() => {
								for (const id of Object.keys(currentSelected)) {
									sync(id);
								}
							}}
							disabled={isAdminReadonly || isSyncing}
						>
							{#if isSyncing}
								<LoaderCircle class="size-4 animate-spin" /> Syncing...
							{:else}
								<RefreshCcw class="size-4" /> Sync
							{/if}
						</button>
						<button
							class="button flex items-center gap-1 text-sm font-normal"
							onclick={() => {
								deletingSources = Object.values(currentSelected);
							}}
							disabled={isAdminReadonly}
						>
							<Trash2 class="size-4" /> Delete
						</button>
					</div>
				{/snippet}
			</Table>
		{:else}
			<div class="my-12 flex w-md flex-col items-center gap-4 self-center text-center">
				<PencilRuler class="text-surface3 size-24" />
				<h4 class="text-on-surface1 text-lg font-semibold">No current Git Source URLs.</h4>
				<p class="text-on-surface1 text-sm font-light">
					Once a Git Source URL has been added, its <br />
					information will be quickly accessible here.
				</p>
			</div>
		{/if}
	</div>
{/snippet}

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
				await AdminService.deleteSkillRepository(source.id);
			}
			skillRepositories = await AdminService.listSkillRepositories();
			skills = await AdminService.listAllSkills();
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
				: 'this'}? This will delete all related skills and their information from the system.
		</p>
	{/snippet}
</Confirm>

<ResponsiveDialog title="Git Source URL Sync" bind:this={syncErrorDialog} class="md:w-2xl">
	<div class="mb-4 flex flex-col gap-4">
		<div class="notification-alert flex flex-col gap-2">
			<div class="flex items-center gap-2">
				<TriangleAlert class="size-6 flex-shrink-0 self-start text-yellow-500" />
				<p class="my-0.5 flex flex-col text-sm font-semibold">
					An issue occurred fetching this source URL:
				</p>
			</div>
			<span class="text-sm font-light break-all">{syncError?.error}</span>
		</div>
	</div>
</ResponsiveDialog>

<dialog bind:this={sourceDialog} class="dialog">
	<div class="dialog-container w-full max-w-md p-4">
		{#if editingSource}
			<h3 class="dialog-title">
				{editingSource.index === -1 ? 'Add Source URL' : 'Edit Source URL'}
				<button onclick={() => closeSourceDialog()} class="icon-button dialog-close-btn">
					<X class="size-5" />
				</button>
			</h3>

			<div class="mt-4 mb-8 flex flex-col gap-4">
				<div class="flex flex-col gap-1">
					<label for="catalog-source-name" class="flex-1 text-sm font-light capitalize"
						>Name
					</label>
					<input
						id="catalog-source-name"
						bind:value={editingSource.name}
						class="text-input-filled"
					/>
				</div>
				<div class="flex flex-col gap-1">
					<label for="catalog-source-url" class="flex-1 text-sm font-light capitalize"
						>Source URL
					</label>
					<input
						id="catalog-source-url"
						bind:value={editingSource.value}
						class="text-input-filled"
					/>
				</div>
				<div class="flex flex-col gap-1">
					<label for="catalog-source-ref" class="flex-1 text-sm font-light capitalize"
						>Reference
					</label>
					<input id="catalog-source-ref" bind:value={editingSource.ref} class="text-input-filled" />
					<span class="text-on-surface1 text-xs"
						>The branch, commit SHA, or tag to index and pull skills from.</span
					>
				</div>
			</div>

			{#if sourceError}
				<div class="mb-4 flex flex-col gap-2 text-red-500 dark:text-red-400">
					<div class="flex items-center gap-2">
						<TriangleAlert class="size-6 flex-shrink-0 self-start" />
						<p class="my-0.5 flex flex-col text-sm font-semibold">Error adding source URL:</p>
					</div>
					<span class="font-sm font-light break-all">{sourceError}</span>
				</div>
			{/if}

			<div class="flex w-full justify-end gap-2">
				<button class="button" disabled={saving} onclick={() => closeSourceDialog()}>Cancel</button>
				<button
					class="button-primary"
					disabled={saving}
					onclick={async () => {
						if (!editingSource) {
							return;
						}

						saving = true;
						sourceError = undefined;

						try {
							const response = await AdminService.createSkillRepository({
								displayName: editingSource.name,
								repoURL: editingSource.value,
								ref: editingSource.ref
							});
							skillRepositories = [...skillRepositories, response];
							sync(response.id);
							closeSourceDialog();
						} catch (error) {
							sourceError = error instanceof Error ? error.message : 'An unexpected error occurred';
						} finally {
							saving = false;
						}
					}}
				>
					Add
				</button>
			</div>
		{/if}
	</div>
	<form class="dialog-backdrop">
		<button type="button" onclick={() => closeSourceDialog()}>close</button>
	</form>
</dialog>

<svelte:head>
	<title>Obot | Admin - Skills</title>
</svelte:head>

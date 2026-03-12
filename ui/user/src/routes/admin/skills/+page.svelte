<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Search from '$lib/components/Search.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { profile } from '$lib/stores';
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
	import { slide } from 'svelte/transition';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { AdminService } from '$lib/services';
	import { SvelteSet, SvelteMap } from 'svelte/reactivity';
	import type { SkillRepository } from '$lib/services/admin/types';
	import { untrack } from 'svelte';
	import { twMerge } from 'tailwind-merge';
	import { formatTimeAgo } from '$lib/time';

	let query = $state('');
	let view = $state<'skills' | 'urls'>('skills');
	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());
	let syncing = new SvelteSet<string>();
	let isSyncing = $derived(syncing.size > 0);

	let deletingSource = $state<SkillRepository | undefined>(undefined);
	let deleting = $state(false);

	let { data } = $props();
	let skillRepositories = $state<SkillRepository[]>(untrack(() => data.skillRepositories));
	let skillRepositoriesTableData = $derived(
		skillRepositories.map((d) => ({
			...d,
			isSyncing: syncing.has(d.id)
		}))
	);
	let skillRepositoriesMap = $derived(new Map(skillRepositories.map((d) => [d.id, d])));
	let skillsTableData = $derived(
		data.skills.map((d) => ({
			...d,
			repository: skillRepositoriesMap.get(d.repoID)?.displayName ?? ''
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
		view = newView;
	}

	function pollTillSyncComplete(id: string) {
		if (syncInterval.get(id)) {
			clearInterval(syncInterval.get(id));
		}

		syncInterval.set(
			id,
			setInterval(async () => {
				const response = await AdminService.getSkillRepository(id);
				if (response && !response.isSyncing) {
					if (syncInterval.get(id)) {
						clearInterval(syncInterval.get(id));
						syncInterval.delete(id);
					}
					skillRepositories = await AdminService.listSkillRepositories();
					syncing.delete(id);
				}
			}, 5000)
		);
	}

	async function sync(id: string) {
		syncing.add(id);
		const response = await AdminService.refreshSkillRepository(id);
		if (!response.isSyncing) {
			syncing.delete(id);
		}
		if (response.isSyncing) {
			pollTillSyncComplete(id);
		}
	}

	function updateSearchQuery(value: string) {
		query = value;
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
						placeholder="Search skills..."
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
		{#if skillsTableData.length > 0}
			<Table
				data={skillsTableData}
				fields={['name', 'created', 'repository']}
				noDataMessage="No skills found."
				columnMaxWidths={{ repository: 320 }}
				classes={{
					root: 'rounded-none rounded-b-md shadow-none'
				}}
				sortable={['name', 'created', 'repository']}
				filterable={['repository']}
				headers={[
					{
						title: 'Name',
						property: 'name'
					},
					{
						title: 'Created',
						property: 'created'
					},
					{
						title: 'Repository',
						property: 'repository'
					}
				]}
			>
				{#snippet onRenderColumn(property, d)}
					{#if property === 'name'}
						{d.name}
					{:else if property === 'created'}
						{formatTimeAgo(d.created).relativeTime}
					{:else if property === 'repository'}
						<span class="block min-w-0 truncate" title={d.repository}>{d.repository}</span>
					{/if}
				{/snippet}
				{#snippet actions(_d)}
					<div></div>
				{/snippet}
			</Table>
		{:else}
			<div class="my-12 flex w-md flex-col items-center gap-4 self-center text-center">
				<PencilRuler class="text-surface3 size-24" />
				<h4 class="text-on-surface1 text-lg font-semibold">No current Git Source URLs.</h4>
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
		{#if skillRepositoriesTableData.length > 0}
			<Table
				data={skillRepositoriesTableData}
				fields={['url']}
				headers={[
					{
						property: 'url',
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
			>
				{#snippet actions(d)}
					{#if !isAdminReadonly}
						<button
							class="icon-button hover:text-red-500"
							onclick={() => {
								deletingSource = d;
							}}
						>
							<Trash2 class="size-4" />
						</button>
					{/if}
				{/snippet}
				{#snippet onRenderColumn(property, d)}
					{#if property === 'url'}
						<div class="flex items-center gap-2">
							<p>{d.repoURL}</p>
							{#if d.syncError}
								<button
									onclick={() => {
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
								for (const id of Object.keys(currentSelected)) {
									AdminService.deleteSkillRepository(id);
								}
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
	msg={`Delete ${deletingSource?.repoURL || 'this Git Source URL'}?`}
	show={Boolean(deletingSource)}
	onsuccess={async () => {
		if (!deletingSource) return;
		deleting = true;
		await AdminService.deleteSkillRepository(deletingSource.id);
		skillRepositories = await AdminService.listSkillRepositories();
		deletingSource = undefined;
		deleting = false;
	}}
	oncancel={() => (deletingSource = undefined)}
	loading={deleting}
>
	{#snippet note()}
		<p>{deletingSource?.repoURL}</p>
		<p>
			Are you sure you want to delete this? This will delete all related skills and their
			information from the system.
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
					<label for="catalog-source-url" class="flex-1 text-sm font-light capitalize"
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
								displayName: editingSource.value,
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
		<button type="button" onclick={() => sourceDialog?.close()}>close</button>
	</form>
</dialog>

<svelte:head>
	<title>Obot | Admin - Skills</title>
</svelte:head>

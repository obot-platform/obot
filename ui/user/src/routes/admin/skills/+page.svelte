<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Search from '$lib/components/Search.svelte';
	import Select from '$lib/components/Select.svelte';
	import SensitiveInput from '$lib/components/SensitiveInput.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { HttpError } from '$lib/errors.js';
	import Loading from '$lib/icons/Loading.svelte';
	import { AdminService } from '$lib/services';
	import type { GitCredential, SkillRepository } from '$lib/services/admin/types';
	import type { Skill } from '$lib/services/nanobot/types';
	import { errors, profile } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import {
		clearUrlParams,
		getTableUrlParamsFilters,
		goto,
		setFilterUrlParams,
		setUrlParamAndUpdateUrl
	} from '$lib/url';
	import { openUrl } from '$lib/utils.js';
	import {
		TriangleAlert,
		Info,
		PencilRuler,
		Pencil,
		Plus,
		RefreshCcw,
		Settings,
		Trash2,
		X,
		GitBranch
	} from '@lucide/svelte';
	import { onDestroy, untrack } from 'svelte';
	import { SvelteSet, SvelteMap } from 'svelte/reactivity';
	import { slide } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	type RepositoryCredentialType = '' | 'shared' | 'token';

	const repositoryCredentialOptions = [
		{ id: 'shared', label: 'Saved Git Credential' },
		{ id: 'token', label: 'Personal Access Token' }
	];

	let { data } = $props();
	let query = $derived(page.url.searchParams.get('query') ?? '');
	let view = $derived<'skills' | 'urls'>(
		((page.url.searchParams.get('view') as 'skills' | 'urls') ?? 'urls') === 'urls'
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
	let gitCredentials = $state<GitCredential[]>(untrack(() => data.gitCredentials ?? []));
	let showLicenseError = $state(untrack(() => data?.showLicenseError ?? false));

	$effect(() => {
		skillRepositories = data.skillRepositories;
	});

	$effect(() => {
		if (view === 'skills') {
			skills = data.skills ?? [];
		}
	});
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
		| {
				index: number;
				value: string;
				name: string;
				ref: string;
				token: string;
				gitCredentialID: string;
				credentialType: RepositoryCredentialType;
				repositoryID?: string;
		  }
		| undefined
	>(undefined);
	let sourceError = $state<string | undefined>(undefined);
	let saving = $state(false);
	let urlFilters = $state(getTableUrlParamsFilters());
	let editingSourceHost = $derived(sourceHost(editingSource?.value ?? ''));
	let gitCredentialOptions = $derived(
		gitCredentials.map((credential) => ({
			id: credential.id,
			label: `${credential.displayName} (${credential.host})`,
			disabled:
				!credential.tokenConfigured ||
				Boolean(editingSourceHost && editingSourceHost !== credential.host.toLowerCase())
		}))
	);
	let editingSkillRepository = $derived(
		editingSource?.repositoryID
			? skillRepositories.find((repository) => repository.id === editingSource?.repositoryID)
			: undefined
	);
	let credentialSelectionIncomplete = $derived(
		Boolean(
			editingSource &&
			((editingSource.credentialType === 'shared' && !editingSource.gitCredentialID) ||
				(editingSource.credentialType === 'token' &&
					!editingSource.token.trim() &&
					(!hasSkillRepositoryToken(editingSkillRepository) ||
						editingSource.value.trim() !== editingSkillRepository?.repoURL)))
		)
	);

	function sourceHost(value: string): string {
		try {
			return new URL(value.includes('://') ? value : `https://${value}`).host.toLowerCase();
		} catch {
			return '';
		}
	}

	function handleSkillSourceURLInput() {
		if (!editingSource?.gitCredentialID) return;
		const selectedCredential = gitCredentials.find(
			(credential) => credential.id === editingSource?.gitCredentialID
		);
		const host = sourceHost(editingSource.value);
		if (selectedCredential && host && host !== selectedCredential.host.toLowerCase()) {
			editingSource.gitCredentialID = '';
		}
	}

	function hasSkillRepositoryToken(repository: SkillRepository | undefined): boolean {
		if (!repository) return false;
		const token = repository.sourceURLCredentials?.[repository.repoURL];
		return token !== undefined && token !== '';
	}

	function switchView(newView: 'skills' | 'urls', filterByRepository: string = '') {
		goto(
			resolve(
				`/admin/skills?view=${newView}${filterByRepository ? `&repository=${encodeURIComponent(filterByRepository)}` : ''}`
			),
			{ replaceState: true }
		);
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
						if (view === 'skills') {
							skills = await AdminService.listAllSkills();
						}
						syncing.delete(id);
					}
				} catch (err) {
					if (err instanceof HttpError && err.statusCode === 402) {
						showLicenseError = true;
					} else {
						errors.append(`Failed to sync skill repository: ${err}`);
					}
					clearSyncInterval(id);
					syncing.delete(id);
				}
			}, 5000)
		);
	}

	function handleFilter(property: string, values: string[]) {
		if (values.length === 0) {
			delete urlFilters[property];
			urlFilters = { ...urlFilters };
		} else {
			urlFilters[property] = values;
		}
		setFilterUrlParams(property, values);
	}

	function handleClearAllFilters() {
		urlFilters = {};
		clearUrlParams();
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
		setUrlParamAndUpdateUrl(page.url, 'query', value);
	}

	function closeSourceDialog() {
		editingSource = undefined;
		sourceError = undefined;
		saving = false;
		sourceDialog?.close();
	}
</script>

<Layout classes={{ navbar: 'bg-base-200' }} title="Skills">
	<div class="flex min-h-full flex-col gap-2">
		<div class="flex min-h-full flex-col">
			<div class="bg-base-200 dark:bg-base-100 sticky top-16 left-0 z-20 w-full py-1">
				<div class="mb-2">
					<Search
						class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
						value={query}
						onChange={updateSearchQuery}
						placeholder={view == 'skills' ? 'Search skills...' : 'Search sources...'}
					/>
				</div>
			</div>
			<div class="dark:bg-base-300 bg-base-100 rounded-t-md shadow-sm">
				<div class="flex">
					<button
						class={twMerge('page-tab max-w-1/2', view === 'urls' && 'page-tab-active')}
						onclick={() => switchView('urls')}
					>
						Sources
					</button>
					<button
						class={twMerge('page-tab max-w-1/2', view === 'skills' && 'page-tab-active')}
						onclick={() => switchView('skills')}
					>
						Skills
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
			<div class="flex items-center gap-2">
				{#if view === 'urls'}
					<button
						class="btn btn-neutral flex items-center gap-1 rounded-4xl text-sm"
						onclick={() => goto('/admin/git-credentials')}
					>
						<Settings class="size-4" />
						Manage Credentials
					</button>
				{/if}
				<button
					class="btn btn-primary flex items-center gap-1 text-sm"
					onclick={() => {
						editingSource = {
							index: -1,
							value: '',
							name: '',
							ref: 'main',
							token: '',
							gitCredentialID: '',
							credentialType: ''
						};
						sourceDialog?.showModal();
					}}
				>
					<Plus class="size-4" /> Add Source URL
				</button>
			</div>
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
				filters={urlFilters}
				onFilter={handleFilter}
				onClearAllFilters={handleClearAllFilters}
			>
				{#snippet onRenderColumn(property, d)}
					{#if property === 'displayName'}
						<span class="flex items-center gap-2">
							{d.displayName}
							{#if d.validationError}
								<div use:tooltip={{ text: d.validationError }}>
									<TriangleAlert class="size-3 text-warning" />
								</div>
							{/if}
						</span>
					{:else if property === 'created'}
						{formatTimeAgo(d.created).relativeTime}
					{:else if property === 'repository'}
						<span class="block min-w-0 truncate">{d.repository}</span>
					{:else if property === 'description'}
						<span class="line-clamp-2 text-sm">{d.description ?? '—'}</span>
					{:else}
						{d[property as keyof typeof d]}
					{/if}
				{/snippet}
				{#snippet actions(d)}
					<a
						class="btn btn-square btn-ghost hover:text-blue-500 btn-sm tooltip tooltip-left"
						href={`${d.repoURL}/tree/${d.repoRef || d.commitSHA || 'main'}/${d.relativePath}`}
						rel="external noopener noreferrer"
						target="_blank"
						onclick={(e) => e.stopPropagation()}
						data-tip="View Source on Git"
					>
						<GitBranch class="size-4" />
					</a>
				{/snippet}
			</Table>
		{:else if showLicenseError}
			<div class="my-12 flex w-md flex-col items-center gap-4 self-center text-center">
				<TriangleAlert class="size-12 text-warning" />
				<h4 class="text-muted-content text-lg font-semibold">License Error</h4>
				<p class="text-muted-content text-sm font-light">
					An issue occurred with fetching skills due to licensing. Please resolve outstanding
					licensing issues or contact support at
					<a href="mailto:info@obot.ai" class="text-link">info@obot.ai</a>.
				</p>
			</div>
		{:else}
			<div class="my-12 flex w-md flex-col items-center gap-4 self-center text-center">
				<PencilRuler class="text-base-content/80 size-24" />
				<h4 class="text-muted-content text-lg font-semibold">No current skills.</h4>
				<p class="text-muted-content text-sm font-light">
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
						return 'bg-warning/10';
					}
					return '';
				}}
				onClickRow={(d) => {
					switchView('skills', d.displayName);
				}}
				classes={{
					root: 'rounded-none rounded-b-md shadow-none'
				}}
				sortable={['displayName']}
			>
				{#snippet actions(d)}
					{#if !isAdminReadonly}
						<IconButton
							onclick={(e) => {
								e.stopPropagation();
								editingSource = {
									index: skillRepositories.findIndex((repository) => repository.id === d.id),
									value: d.repoURL,
									name: d.displayName,
									ref: d.ref,
									token: '',
									gitCredentialID: d.gitCredentialID ?? '',
									credentialType: d.gitCredentialID
										? 'shared'
										: hasSkillRepositoryToken(d)
											? 'token'
											: '',
									repositoryID: d.id
								};
								sourceDialog?.showModal();
							}}
						>
							<Pencil class="size-4" />
						</IconButton>
						<IconButton
							variant="danger"
							onclick={(e) => {
								e.stopPropagation();
								deletingSources = [d];
							}}
						>
							<Trash2 class="size-4" />
						</IconButton>
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
										classes: ['wrap-break-word']
									}}
								>
									<TriangleAlert class="size-4 text-warning" />
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
							class="btn btn-secondary flex items-center gap-1 text-sm font-normal"
							onclick={() => {
								for (const id of Object.keys(currentSelected)) {
									sync(id);
								}
							}}
							disabled={isAdminReadonly || isSyncing}
						>
							{#if isSyncing}
								<Loading class="size-4" /> Syncing...
							{:else}
								<RefreshCcw class="size-4" /> Sync
							{/if}
						</button>
						<button
							class="btn btn-secondary flex items-center gap-1 text-sm font-normal"
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
				<PencilRuler class="text-muted-content size-24 opacity-25" />
				<h4 class="text-muted-content text-lg font-semibold">No current Git Source URLs.</h4>
				<p class="text-muted-content text-sm font-light">
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
			if (view === 'skills') {
				skills = await AdminService.listAllSkills();
			}
		} catch (error) {
			if (error instanceof HttpError && error.statusCode === 402) {
				showLicenseError = true;
			} else {
				errors.append(`Failed to delete Git Source URLs: ${error}`);
			}
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
				<TriangleAlert class="size-6 shrink-0 self-start text-warning" />
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
				<IconButton onclick={() => closeSourceDialog()} class="btn-sm dialog-close-btn">
					<X class="size-5" />
				</IconButton>
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
						oninput={handleSkillSourceURLInput}
						class="text-input-filled"
					/>
				</div>
				<div class="flex flex-col gap-1">
					<label for="catalog-source-ref" class="flex-1 text-sm font-light capitalize"
						>Reference
					</label>
					<input id="catalog-source-ref" bind:value={editingSource.ref} class="text-input-filled" />
					<span class="text-muted-content text-xs"
						>The branch, commit SHA, or tag to index and pull skills from.</span
					>
				</div>
				<div class="flex flex-col gap-1">
					<Select
						id="skill-source-credential-type"
						options={repositoryCredentialOptions}
						selected={editingSource.credentialType}
						placeholder="Repository Credential (optional)"
						class={!editingSource.credentialType ? 'text-muted-content' : undefined}
						onSelect={(option) => {
							if (!editingSource) return;
							editingSource.credentialType = option.id as RepositoryCredentialType;
							if (option.id === 'shared') {
								editingSource.token = '';
							} else {
								editingSource.gitCredentialID = '';
							}
						}}
						onClear={editingSource.credentialType
							? () => {
									if (!editingSource) return;
									editingSource.credentialType = '';
									editingSource.gitCredentialID = '';
									editingSource.token = '';
								}
							: undefined}
					/>
				</div>
				{#if editingSource.credentialType === 'shared'}
					<div class="flex flex-col gap-1">
						<label for="skill-source-git-credential" class="text-sm font-light">
							Saved Git credential
						</label>
						<Select
							id="skill-source-git-credential"
							options={gitCredentialOptions}
							selected={editingSource.gitCredentialID}
							placeholder={gitCredentials.length
								? 'Select a saved credential'
								: 'No saved credentials'}
							searchInDropdown
							onSelect={(option) => {
								if (editingSource) {
									editingSource.gitCredentialID = String(option.id);
									editingSource.token = '';
								}
							}}
							onClear={editingSource.gitCredentialID
								? () => {
										if (editingSource) editingSource.gitCredentialID = '';
									}
								: undefined}
						/>
						<span class="text-muted-content text-xs">
							Only credentials matching the repository host can be selected.
						</span>
					</div>
				{/if}
				{#if editingSource.credentialType === 'token'}
					<div class="flex flex-col gap-1">
						<label for="skill-source-token" class="flex items-center gap-1 text-sm font-light">
							Personal Access Token
						</label>
						<SensitiveInput name="skill-source-token" bind:value={editingSource.token} />
					</div>
				{/if}
			</div>

			{#if sourceError}
				<div class="mb-4 flex flex-col gap-2 text-error">
					<div class="flex items-center gap-2">
						<TriangleAlert class="size-6 shrink-0 self-start" />
						<p class="my-0.5 flex flex-col text-sm font-semibold">Error saving source URL:</p>
					</div>
					<span class="font-sm font-light break-all">{sourceError}</span>
				</div>
			{/if}

			<div class="flex w-full justify-end gap-2">
				<button class="btn btn-secondary" disabled={saving} onclick={() => closeSourceDialog()}
					>Cancel</button
				>
				<button
					class="btn btn-primary"
					disabled={saving || credentialSelectionIncomplete}
					onclick={async () => {
						if (!editingSource) {
							return;
						}

						saving = true;
						sourceError = undefined;

						try {
							const repoURL = editingSource.value.trim();
							const token = editingSource.token.trim();
							const manifest: Parameters<typeof AdminService.createSkillRepository>[0] = {
								displayName: editingSource.name,
								repoURL,
								ref: editingSource.ref
							};
							if (editingSource.gitCredentialID) {
								manifest.gitCredentialID = editingSource.gitCredentialID;
							} else if (editingSource.credentialType === 'token' && token) {
								manifest.sourceURLCredentials = { [repoURL]: token };
							} else if (
								editingSource.credentialType !== 'token' &&
								hasSkillRepositoryToken(
									skillRepositories.find(
										(repository) => repository.id === editingSource?.repositoryID
									)
								)
							) {
								manifest.sourceURLCredentials = { [repoURL]: '' };
							}
							const response = editingSource.repositoryID
								? await AdminService.updateSkillRepository(editingSource.repositoryID, manifest)
								: await AdminService.createSkillRepository(manifest);
							skillRepositories = editingSource.repositoryID
								? skillRepositories.map((repository) =>
										repository.id === response.id ? response : repository
									)
								: [...skillRepositories, response];
							sync(response.id);
							closeSourceDialog();
						} catch (error) {
							sourceError = error instanceof Error ? error.message : 'An unexpected error occurred';
						} finally {
							saving = false;
						}
					}}
				>
					{editingSource.repositoryID ? 'Save' : 'Add'}
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

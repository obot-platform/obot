<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Confirm from '$lib/components/Confirm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Select from '$lib/components/Select.svelte';
	import SensitiveInput from '$lib/components/SensitiveInput.svelte';
	import TabLayout, { type TabView } from '$lib/components/TabLayout.svelte';
	import SkillAccessPolicyForm from '$lib/components/admin/SkillAccessPolicyForm.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { HttpError, parseErrorContent } from '$lib/errors.js';
	import { AdminService } from '$lib/services';
	import type {
		GitCredential,
		SkillAccessPolicy,
		SkillRepository
	} from '$lib/services/admin/types';
	import type { Skill } from '$lib/services/nanobot/types';
	import { errors, profile } from '$lib/stores';
	import { clearUrlParams, getTableUrlParamsFilters, goto, setFilterUrlParams } from '$lib/url';
	import SkillsPoliciesView from './SkillsPoliciesView.svelte';
	import SkillsView from './SkillsView.svelte';
	import SourcesView from './SourcesView.svelte';
	import { Info, Plus, Settings, TriangleAlert, X } from '@lucide/svelte';
	import { onDestroy, untrack } from 'svelte';
	import { SvelteMap, SvelteSet } from 'svelte/reactivity';
	import { fly, slide } from 'svelte/transition';

	type RepositoryCredentialType = 'none' | 'shared' | 'token';

	const repositoryCredentialOptions = [
		{ id: 'none', label: 'None' },
		{ id: 'shared', label: 'Choose existing' },
		{ id: 'token', label: 'Enter personal access token' }
	];

	let { data } = $props();
	let isAdminReadonly = $derived(Boolean(profile.current.isAdminReadonly?.()));
	let hasAdminAccess = $derived(Boolean(profile.current.hasAdminAccess?.()));
	const viewValues = ['skills', 'sources', 'git-credentials', 'access-policies'] as const;
	let selectedView = $derived.by(() => {
		const requested = page.url.searchParams.get('view');
		return requested && viewValues.includes(requested as (typeof viewValues)[number])
			? requested
			: 'skills';
	});
	let creating = $derived(
		hasAdminAccess &&
			!isAdminReadonly &&
			selectedView === 'access-policies' &&
			page.url.searchParams.has('new')
	);
	const duration = PAGE_TRANSITION_DURATION;
	let layoutTitle = $derived(creating ? 'Create Skill Access Policy' : 'Skills');
	let views = $derived.by(() => {
		const items: TabView[] = [{ label: 'Skills', value: 'skills', content: skillsView }];
		if (hasAdminAccess) {
			items.push(
				{ label: 'Sources', value: 'sources', content: sourcesView },
				{ label: 'Access Policies', value: 'access-policies', content: accessPolicy }
			);
		}
		return items;
	});
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
		gitCredentials = data.gitCredentials ?? [];
	});

	$effect(() => {
		if (selectedView === 'skills') {
			skills = data.skills ?? [];
		}
	});

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
				clearToken?: boolean;
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
	let existingSkillRepositoryToken = $derived(
		editingSource?.value.trim() === editingSkillRepository?.repoURL
			? (editingSkillRepository?.sourceURLCredentials?.[editingSkillRepository.repoURL] ?? '')
			: ''
	);
	let existingSourceHasCredential = $derived(
		Boolean(
			editingSource &&
			editingSource.index >= 0 &&
			(hasSkillRepositoryToken(editingSkillRepository) ||
				Boolean(editingSkillRepository?.gitCredentialID))
		)
	);
	let credentialLocked = $derived(
		Boolean(editingSource && existingSourceHasCredential && !editingSource.clearToken)
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
						if (selectedView === 'skills') {
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

	function closeSourceDialog() {
		editingSource = undefined;
		sourceError = undefined;
		saving = false;
		sourceDialog?.close();
	}

	function openAddSource() {
		editingSource = {
			index: -1,
			value: '',
			name: '',
			ref: 'main',
			token: '',
			gitCredentialID: '',
			credentialType: 'none'
		};
		sourceDialog?.showModal();
	}

	function openEditSource(repository: SkillRepository) {
		editingSource = {
			index: skillRepositories.findIndex((candidate) => candidate.id === repository.id),
			value: repository.repoURL,
			name: repository.displayName,
			ref: repository.ref,
			token: '',
			gitCredentialID: repository.gitCredentialID ?? '',
			credentialType: repository.gitCredentialID
				? 'shared'
				: hasSkillRepositoryToken(repository)
					? 'token'
					: 'none',
			repositoryID: repository.id
		};
		sourceDialog?.showModal();
	}

	function openSkillsForRepository(displayName: string) {
		urlFilters = { repository: [displayName] };
		goto(`${page.url.pathname}?view=skills&repository=${encodeURIComponent(displayName)}`);
	}

	function closeCreateScreen() {
		goto(`${page.url.pathname}?view=${selectedView}`);
	}

	function navigateToCreated(policy: SkillAccessPolicy) {
		clearUrlParams(['new']);
		goto(`/skills/access-policies/${policy.id}`, { replaceState: false });
	}

	onDestroy(() => {
		for (const interval of syncInterval.values()) {
			clearInterval(interval);
		}
	});
</script>

<svelte:head>
	<title>Obot | {layoutTitle}</title>
</svelte:head>

{#if creating}
	<Layout title={layoutTitle} showBackButton onBackButtonClick={closeCreateScreen}>
		<div class="h-full w-full" in:fly|global={{ x: 100, delay: duration, duration }}>
			<SkillAccessPolicyForm onCreate={navigateToCreated} readonly={isAdminReadonly} />
		</div>
	</Layout>
{:else}
	<TabLayout
		title={layoutTitle}
		defaultView="skills"
		rightNavActions={navActions}
		{views}
		classes={{ childrenContainer: 'max-w-none' }}
	/>
{/if}

{#snippet navActions(view: string)}
	{#if hasAdminAccess}
		{#if !isAdminReadonly && view === 'access-policies'}
			<button
				class="btn btn-primary flex items-center gap-1 text-sm"
				onclick={() => goto(`${page.url.pathname}?view=access-policies&new=true`)}
			>
				<Plus class="size-4" /> Add Access Policy
			</button>
		{:else if !isAdminReadonly && (view === 'skills' || view === 'sources')}
			<a
				class="btn btn-secondary flex items-center gap-1 text-sm"
				href={resolve('/admin/platform?view=git-credentials')}
			>
				<Settings class="size-4" /> Manage Credentials
			</a>
			<button class="btn btn-primary flex items-center gap-1 text-sm" onclick={openAddSource}>
				<Plus class="size-4" /> Add Source URL
			</button>
		{/if}
	{/if}
{/snippet}

{#snippet skillsView()}
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
	<SkillsView
		{skills}
		{skillRepositories}
		{showLicenseError}
		{urlFilters}
		onFilter={handleFilter}
		onClearAllFilters={handleClearAllFilters}
	/>
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
		{skillRepositories}
		syncingIds={syncing}
		{isAdminReadonly}
		onEdit={openEditSource}
		onDelete={(repositories) => (deletingSources = repositories)}
		onSync={sync}
		onOpenSyncError={(url, error) => {
			syncError = { url, error };
			syncErrorDialog?.open();
		}}
		onSelectRepository={openSkillsForRepository}
	/>
{/snippet}

{#snippet accessPolicy()}
	<SkillsPoliciesView skillAccessPolicies={data.skillAccessPolicies} />
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
			if (selectedView === 'skills') {
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
	<div class="dialog-container w-full max-w-md p-4 h-134.5 max-h-dvh flex flex-col">
		{#if editingSource}
			<h3 class="dialog-title">
				{editingSource.index === -1 ? 'Add Source URL' : 'Edit Source URL'}
				<IconButton onclick={() => closeSourceDialog()} class="btn-sm dialog-close-btn">
					<X class="size-5" />
				</IconButton>
			</h3>

			<div class="flex flex-col gap-4">
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
				<div class="flex flex-col gap-2">
					<div class="flex flex-col gap-1">
						<div class="flex items-center justify-between gap-4">
							<span id="skill-source-credential-label" class="flex-1 text-sm font-light capitalize">
								Credential
							</span>
							{#if credentialLocked}
								<div class="flex justify-end">
									<button
										class="text-xs text-error hover:underline"
										onclick={() => {
											if (!editingSource) return;
											editingSource.credentialType = 'none';
											editingSource.gitCredentialID = '';
											editingSource.token = '';
											editingSource.clearToken = true;
										}}
									>
										Clear token
									</button>
								</div>
							{/if}
						</div>
						<Select
							id="skill-source-credential-type"
							class="bg-base-200"
							options={repositoryCredentialOptions}
							selected={editingSource.credentialType}
							ariaLabelledby="skill-source-credential-label"
							disabled={credentialLocked}
							onSelect={(option) => {
								if (!editingSource || credentialLocked) return;
								editingSource.credentialType = option.id as RepositoryCredentialType;
								if (option.id === 'shared') {
									editingSource.token = '';
								} else if (option.id === 'token') {
									editingSource.gitCredentialID = '';
								} else {
									editingSource.gitCredentialID = '';
									editingSource.token = '';
									if (hasSkillRepositoryToken(editingSkillRepository)) {
										editingSource.clearToken = true;
									}
								}
							}}
						/>
					</div>
					{#if editingSource.credentialType === 'shared'}
						<div class="flex flex-col gap-1">
							<Select
								id="skill-source-git-credential"
								class="bg-base-200"
								options={gitCredentialOptions}
								selected={editingSource.gitCredentialID}
								searchPlaceholder=""
								searchInDropdown
								disabled={credentialLocked}
								onSelect={(option) => {
									if (!editingSource || credentialLocked) return;
									editingSource.gitCredentialID = String(option.id);
									editingSource.token = '';
								}}
								onClear={!credentialLocked && editingSource.gitCredentialID
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
							<label for="skill-source-token" class="sr-only">Personal Access Token</label>
							{#if credentialLocked && existingSkillRepositoryToken}
								<input
									id="skill-source-token"
									type="text"
									readonly
									aria-readonly="true"
									data-1p-ignore
									value={existingSkillRepositoryToken}
									class="text-sm text-muted-content w-full border-none bg-transparent p-0 outline-none focus:ring-0 min-h-10"
								/>
							{:else}
								<SensitiveInput
									name="skill-source-token"
									placeholder="Personal Access Token"
									bind:value={editingSource.token}
								/>
							{/if}
						</div>
					{/if}
				</div>
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

			<div class="flex grow mb-4"></div>

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
								!token &&
								(editingSource.clearToken ||
									(editingSource.credentialType !== 'token' &&
										hasSkillRepositoryToken(editingSkillRepository)))
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
							sourceError = parseErrorContent(error).message;
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

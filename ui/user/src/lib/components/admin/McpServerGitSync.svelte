<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Select from '$lib/components/Select.svelte';
	import SensitiveInput from '$lib/components/SensitiveInput.svelte';
	import {
		AdminService,
		type GitCredential,
		type MCPCatalog,
		type MCPCatalogManifest
	} from '$lib/services';
	import IconButton from '../primitives/IconButton.svelte';
	import { Info, TriangleAlert, X } from '@lucide/svelte';
	import { slide } from 'svelte/transition';

	interface Props {
		defaultCatalog?: MCPCatalog;
		defaultCatalogId?: string;
		onSync?: () => void;
		gitCredentials?: GitCredential[];
	}

	type RepositoryCredentialType = 'none' | 'shared' | 'token';

	const repositoryCredentialOptions = [
		{ id: 'none', label: 'None' },
		{ id: 'shared', label: 'Choose existing' },
		{ id: 'token', label: 'Enter personal access token' }
	];

	let { defaultCatalog, onSync, defaultCatalogId, gitCredentials = [] }: Props = $props();

	let saving = $state(false);
	let sourceError = $state<string>();
	let editingSource = $state<{
		index: number;
		value: string;
		token: string;
		gitCredentialID: string;
		credentialType: RepositoryCredentialType;
		clearToken?: boolean;
	}>();
	let sourceDialog = $state<HTMLDialogElement>();
	let tokenClearedForURLChange = $state(false);
	let tokenExplicitlyCleared = $state(false);

	export function open() {
		sourceError = undefined;
		tokenClearedForURLChange = false;
		tokenExplicitlyCleared = false;
		editingSource = {
			index: -1,
			value: '',
			token: '',
			gitCredentialID: '',
			credentialType: 'none'
		};
		sourceDialog?.showModal();
	}

	export function edit(url: string, index: number) {
		sourceError = undefined;
		tokenClearedForURLChange = false;
		tokenExplicitlyCleared = false;
		editingSource = {
			index,
			value: url,
			token: '',
			gitCredentialID: defaultCatalog?.sourceURLGitCredentialIDs?.[url] ?? '',
			credentialType: defaultCatalog?.sourceURLGitCredentialIDs?.[url]
				? 'shared'
				: hasSourceURLCredential(url)
					? 'token'
					: 'none'
		};
		sourceDialog?.showModal();
	}

	function closeSourceDialog() {
		editingSource = undefined;
		sourceError = undefined;
		tokenClearedForURLChange = false;
		tokenExplicitlyCleared = false;
		sourceDialog?.close();
	}

	function hasSourceURLCredential(url: string | undefined, catalog = defaultCatalog): boolean {
		if (!url) {
			return false;
		}
		const credential = catalog?.sourceURLCredentials?.[url];
		return credential !== undefined && credential !== '';
	}

	const editingSourceURL = $derived(
		editingSource && editingSource.index >= 0
			? defaultCatalog?.sourceURLs?.[editingSource.index]
			: undefined
	);

	const existingSourceHasCredential = $derived(
		Boolean(
			editingSource &&
			editingSource.index >= 0 &&
			editingSourceURL &&
			(hasSourceURLCredential(editingSourceURL) ||
				Boolean(defaultCatalog?.sourceURLGitCredentialIDs?.[editingSourceURL]))
		)
	);

	const credentialLocked = $derived(
		Boolean(editingSource && existingSourceHasCredential && !editingSource.clearToken)
	);

	const sourceURLChangedWithCredential = $derived(
		Boolean(
			editingSource &&
			editingSource.index >= 0 &&
			editingSourceURL &&
			editingSource.value !== editingSourceURL &&
			hasSourceURLCredential(editingSourceURL, defaultCatalog) &&
			!editingSource.token
		)
	);
	const credentialSelectionIncomplete = $derived(
		Boolean(
			editingSource &&
			((editingSource.credentialType === 'shared' && !editingSource.gitCredentialID) ||
				(editingSource.credentialType === 'token' &&
					!editingSource.token.trim() &&
					(editingSource.clearToken ||
						editingSource.value !== editingSourceURL ||
						!hasSourceURLCredential(editingSourceURL))))
		)
	);
	const editingSourceHost = $derived(sourceHost(editingSource?.value ?? ''));
	const gitCredentialOptions = $derived(
		gitCredentials.map((credential) => ({
			id: credential.id,
			label: `${credential.displayName} (${credential.host})`,
			disabled:
				!credential.tokenConfigured ||
				Boolean(editingSourceHost && editingSourceHost !== credential.host.toLowerCase())
		}))
	);

	function sourceHost(value: string): string {
		try {
			return new URL(value.includes('://') ? value : `https://${value}`).host.toLowerCase();
		} catch {
			return '';
		}
	}

	function handleSourceURLInput() {
		if (!editingSource) {
			return;
		}

		const selectedCredentialID = editingSource.gitCredentialID;
		const selectedCredential = gitCredentials.find(
			(credential) => credential.id === selectedCredentialID
		);
		const host = sourceHost(editingSource.value);
		if (selectedCredential && host && host !== selectedCredential.host.toLowerCase()) {
			editingSource.gitCredentialID = '';
		}
		if (editingSource.index < 0 || !editingSourceURL) {
			return;
		}

		const urlChanged = editingSource.value !== editingSourceURL;
		const hadCredential = hasSourceURLCredential(editingSourceURL, defaultCatalog);

		if (urlChanged && hadCredential) {
			editingSource.clearToken = true;
			if (!tokenClearedForURLChange) {
				editingSource.token = '';
				tokenClearedForURLChange = true;
			}
		} else if (!urlChanged && tokenClearedForURLChange && !tokenExplicitlyCleared) {
			editingSource.clearToken = false;
			editingSource.token = '';
			tokenClearedForURLChange = false;
		}
	}
</script>

{#snippet tokenScopesTooltip()}
	<div class="text-left">
		<p>Required scopes:</p>
		<ul class="list-disc pl-4">
			<li>GitHub: repo</li>
			<li>GitLab: read_repository + read_api</li>
		</ul>
		<p class="mt-2">
			If no token is set, Obot falls back to the GITHUB_AUTH_TOKEN environment variable.
		</p>
	</div>
{/snippet}

<dialog bind:this={sourceDialog} class="dialog">
	<div class="dialog-container w-full max-w-md p-4 h-91.5 max-h-dvh flex flex-col">
		{#if editingSource}
			<h3 class="dialog-title">
				{editingSource.index === -1 ? 'Add Source URL' : 'Edit Source URL'}
				<IconButton onclick={closeSourceDialog} class="btn-sm dialog-close-btn">
					<X class="size-5" />
				</IconButton>
			</h3>

			<div class="mb-4 flex flex-col gap-1">
				<label for="catalog-source-name" class="flex flex-1 items-center gap-1 text-sm font-light">
					Source URL
					<span
						use:tooltip={{
							text: 'Supported formats:\n• https://github.com/org/repo\n• https://github.com/org/repo/my-branch\n• https://gitlab.com/org/repo\n• https://gitlab.com/org/repo/my-branch\n• https://gitlab.com/group/subgroup/repo.git\n• https://self-hosted.example.com/org/repo.git\n\nFor GitHub and GitLab a .git suffix is optional. For self-hosted instances it is required.\nGitLab subgroup repos require the .git suffix.',
							classes: ['max-w-md', 'whitespace-pre-line'],
							disablePortal: true
						}}
					>
						<Info class="text-muted-content size-3.5" />
					</span>
				</label>
				<input
					id="catalog-source-name"
					bind:value={editingSource.value}
					oninput={handleSourceURLInput}
					class="text-input-filled"
				/>
			</div>

			<div class="mb-2 flex flex-col gap-1">
				<div class="flex items-center justify-between gap-4">
					<span id="catalog-source-credential-label" class="flex-1 text-sm font-light capitalize">
						Credential
					</span>
					{#if credentialLocked}
						<button
							class="text-xs text-error hover:underline"
							onclick={() => {
								if (!editingSource) return;
								editingSource.credentialType = 'none';
								editingSource.gitCredentialID = '';
								editingSource.token = '';
								editingSource.clearToken = true;
								tokenExplicitlyCleared = true;
							}}
						>
							Clear token
						</button>
					{/if}
				</div>
				<Select
					id="catalog-source-credential-type"
					class="bg-base-200"
					options={repositoryCredentialOptions}
					selected={editingSource.credentialType}
					ariaLabelledby="catalog-source-credential-label"
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
							if (hasSourceURLCredential(editingSourceURL)) {
								editingSource.clearToken = true;
								tokenExplicitlyCleared = true;
							}
						}
					}}
				/>
			</div>

			{#if editingSource.credentialType === 'shared'}
				<div class="mb-4 flex flex-col gap-1">
					<Select
						id="catalog-source-git-credential"
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
							editingSource.clearToken = false;
						}}
						onClear={!credentialLocked && editingSource.gitCredentialID
							? () => {
									if (editingSource) editingSource.gitCredentialID = '';
								}
							: undefined}
					/>
					<span class="text-muted-content text-xs">
						Only credentials matching the source host can be selected.
					</span>
				</div>
			{/if}

			{#if editingSource.credentialType === 'token'}
				<div class="mb-4 flex flex-col gap-1">
					<label for="catalog-source-token" class="sr-only">Personal Access Token</label>
					<div class="flex items-center gap-2 min-h-10">
						{#if credentialLocked && hasSourceURLCredential(editingSourceURL)}
							<input
								id="catalog-source-token"
								type="text"
								readonly
								aria-readonly="true"
								data-1p-ignore
								value={defaultCatalog?.sourceURLCredentials?.[editingSourceURL ?? ''] ?? ''}
								class="text-sm text-muted-content w-full border-none bg-transparent p-0 outline-none focus:ring-0"
							/>
						{:else}
							<SensitiveInput
								name="catalog-source-token"
								placeholder="Personal Access Token"
								bind:value={editingSource.token}
							/>
						{/if}
						<span
							use:tooltip={{
								snippet: tokenScopesTooltip,
								classes: ['max-w-md'],
								disablePortal: true
							}}
						>
							<Info class="text-muted-content size-3.5" />
						</span>
					</div>
				</div>
			{/if}

			{#if sourceError}
				<div class="mb-4 flex flex-col gap-2 text-error">
					<div class="flex items-center gap-2">
						<TriangleAlert class="size-6 shrink-0 self-start" />
						<p class="my-0.5 flex flex-col text-sm font-semibold">Error adding source URL:</p>
					</div>
					<span class="font-sm font-light break-all">{sourceError}</span>
				</div>
			{:else if sourceURLChangedWithCredential && !tokenExplicitlyCleared}
				<p class="mb-4 text-xs notification-alert" in:slide={{ axis: 'y' }}>
					The source URL has been changed. Please re-enter the personal access token tied to the
					former URL, otherwise it will be cleared on save.
				</p>
			{/if}

			<div class="flex grow mb-4"></div>

			<div class="flex w-full justify-end gap-2">
				<button class="btn btn-secondary" disabled={saving} onclick={closeSourceDialog}
					>Cancel</button
				>
				<button
					class="btn btn-primary"
					disabled={saving || credentialSelectionIncomplete}
					onclick={async () => {
						if (!editingSource || (!defaultCatalog && !defaultCatalogId)) {
							return;
						}

						let catalogToUse = defaultCatalog;
						if (!catalogToUse && defaultCatalogId) {
							catalogToUse = await AdminService.getMCPCatalog(defaultCatalogId);
						}

						if (!catalogToUse) {
							sourceError = 'Failed to fetch catalog';
							return;
						}

						saving = true;
						sourceError = undefined;

						try {
							const updatingCatalog: MCPCatalogManifest = {
								displayName: catalogToUse.displayName,
								sourceURLs: catalogToUse.sourceURLs ?? [],
								allowedUserIDs: catalogToUse.allowedUserIDs
							};
							const oldURL =
								editingSource.index >= 0
									? catalogToUse.sourceURLs?.[editingSource.index]
									: undefined;
							const newURL = editingSource.value;

							if (editingSource.index === -1) {
								updatingCatalog.sourceURLs = [...(updatingCatalog.sourceURLs ?? []), newURL];
							} else {
								updatingCatalog.sourceURLs = [...(updatingCatalog.sourceURLs ?? [])];
								updatingCatalog.sourceURLs[editingSource.index] = newURL;
							}

							const sourceURLCredentials: Record<string, string> = {};

							if (
								oldURL !== undefined &&
								oldURL !== newURL &&
								hasSourceURLCredential(oldURL, catalogToUse)
							) {
								sourceURLCredentials[oldURL] = '';
							}

							if (
								!editingSource.token &&
								(editingSource.clearToken ||
									(editingSource.credentialType !== 'token' &&
										hasSourceURLCredential(oldURL, catalogToUse)))
							) {
								sourceURLCredentials[newURL] = '';
							} else if (editingSource.token) {
								sourceURLCredentials[newURL] = editingSource.token;
							}

							if (Object.keys(sourceURLCredentials).length > 0) {
								updatingCatalog.sourceURLCredentials = sourceURLCredentials;
							}

							const sourceURLGitCredentialIDs = {
								...(catalogToUse.sourceURLGitCredentialIDs ?? {})
							};
							if (oldURL && oldURL !== newURL) {
								delete sourceURLGitCredentialIDs[oldURL];
							}
							if (editingSource.gitCredentialID) {
								sourceURLGitCredentialIDs[newURL] = editingSource.gitCredentialID;
							} else {
								delete sourceURLGitCredentialIDs[newURL];
							}
							updatingCatalog.sourceURLGitCredentialIDs = sourceURLGitCredentialIDs;

							const response = await AdminService.updateMCPCatalog(
								catalogToUse.id,
								updatingCatalog,
								{
									dontLogErrors: true
								}
							);
							defaultCatalog = response;
							await onSync?.();
							closeSourceDialog();
						} catch (error) {
							sourceError = error instanceof Error ? error.message : 'An unexpected error occurred';
						} finally {
							saving = false;
						}
					}}
				>
					{editingSource.index === -1 ? 'Add' : 'Save'}
				</button>
			</div>
		{/if}
	</div>
	<form class="dialog-backdrop">
		<button type="button" onclick={closeSourceDialog}>close</button>
	</form>
</dialog>

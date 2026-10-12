<script lang="ts">
	import { resolve } from '$app/paths';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Select from '$lib/components/Select.svelte';
	import SensitiveInput from '$lib/components/SensitiveInput.svelte';
	import { m } from '$lib/i18n';
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
		{ id: 'none', label: m.core_col_none() },
		{ id: 'shared', label: m.mcps_sources_cred_choose_existing() },
		{ id: 'token', label: m.mcps_sources_cred_enter_pat() }
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
		<p>{m.mcps_sources_required_scopes()}</p>
		<ul class="list-disc pl-4">
			<li>GitHub: repo</li>
			<li>GitLab: read_repository + read_api</li>
		</ul>
		<p class="mt-2">
			{m.mcps_sources_token_fallback_env()}
		</p>
	</div>
{/snippet}

<dialog bind:this={sourceDialog} class="dialog">
	<div class="dialog-container w-full max-w-md p-4 h-96 max-h-dvh flex flex-col">
		{#if editingSource}
			<h3 class="dialog-title">
				{editingSource.index === -1
					? m.mcps_sources_add_source_url()
					: m.mcps_sources_edit_source_url()}
				<IconButton onclick={closeSourceDialog} class="btn-sm dialog-close-btn">
					<X class="size-5" />
				</IconButton>
			</h3>

			<div class="mb-4 flex flex-col gap-1">
				<label for="catalog-source-name" class="flex flex-1 items-center gap-1 text-sm font-light">
					{m.mcps_sources_source_url()}
					<span
						use:tooltip={{
							text: m.mcps_sources_source_url_formats_tooltip(),
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
						{m.core_col_credential()}
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
							{m.mcps_sources_clear_token()}
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
				<p class="text-xs text-muted-content font-light">
					{m.mcps_sources_need_modify_credential()}
					<a
						class="text-blue-500 hover:underline"
						href={resolve('/admin/platform?view=settings#git-credentials')}
						>{m.mcps_sources_manage_credentials()}</a
					>
				</p>
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
						{m.mcps_sources_only_matching_host_credentials()}
					</span>
				</div>
			{/if}

			{#if editingSource.credentialType === 'token'}
				<div class="mb-4 flex flex-col gap-1">
					<label for="catalog-source-token" class="sr-only"
						>{m.mcps_sources_personal_access_token()}</label
					>
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
								placeholder={m.mcps_sources_personal_access_token()}
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
						<p class="my-0.5 flex flex-col text-sm font-semibold">
							{m.mcps_sources_error_adding_source_url()}
						</p>
					</div>
					<span class="font-sm font-light break-all">{sourceError}</span>
				</div>
			{:else if sourceURLChangedWithCredential && !tokenExplicitlyCleared}
				<p class="mb-4 text-xs notification-alert" in:slide={{ axis: 'y' }}>
					{m.mcps_sources_source_url_changed_notice()}
				</p>
			{/if}

			<div class="flex grow mb-4"></div>

			<div class="flex w-full justify-end gap-2">
				<button class="btn btn-secondary" disabled={saving} onclick={closeSourceDialog}
					>{m.common_cancel()}</button
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
							sourceError = m.mcps_sources_failed_fetch_catalog();
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
							sourceError =
								error instanceof Error ? error.message : m.mcps_sources_unexpected_error();
						} finally {
							saving = false;
						}
					}}
				>
					{editingSource.index === -1 ? m.mcps_sources_add() : m.core_save()}
				</button>
			</div>
		{/if}
	</div>
	<form class="dialog-backdrop">
		<button type="button" onclick={closeSourceDialog}>{m.common_close()}</button>
	</form>
</dialog>

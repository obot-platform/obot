<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import SensitiveInput from '$lib/components/SensitiveInput.svelte';
	import { AdminService, type MCPCatalog, type MCPCatalogManifest } from '$lib/services';
	import IconButton from '../primitives/IconButton.svelte';
	import { Info, TriangleAlert, X } from '@lucide/svelte';
	import { slide } from 'svelte/transition';

	interface Props {
		defaultCatalog?: MCPCatalog;
		defaultCatalogId?: string;
		onSync?: () => void;
	}

	let { defaultCatalog, onSync, defaultCatalogId }: Props = $props();

	let saving = $state(false);
	let sourceError = $state<string>();
	let editingSource = $state<{
		index: number;
		value: string;
		token: string;
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
			token: ''
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
			token: ''
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

	function handleSourceURLInput() {
		if (!editingSource || editingSource.index < 0 || !editingSourceURL) {
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

<dialog bind:this={sourceDialog} class="dialog">
	<div class="dialog-container w-full max-w-md p-4">
		{#if editingSource}
			<h3 class="dialog-title">
				{editingSource.index === -1 ? 'Add Source URL' : 'Edit Source URL'}
				<IconButton onclick={closeSourceDialog} class="btn-sm dialog-close-btn">
					<X class="size-5" />
				</IconButton>
			</h3>

			<div class="my-4 flex flex-col gap-1">
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

			<div class="mb-4 flex flex-col gap-1">
				<div class="flex items-center justify-between">
					<label for="catalog-source-token" class="flex items-center gap-1 text-sm font-light">
						Personal access token (optional)
						<span
							use:tooltip={{
								text: 'Required scopes:\n• GitHub: repo\n• GitLab: read_repository + read_api\n\nIf no token is set, Obot falls back to the GITHUB_AUTH_TOKEN environment variable.',
								classes: ['max-w-md', 'whitespace-pre-line'],
								disablePortal: true
							}}
						>
							<Info class="text-muted-content size-3.5" />
						</span>
					</label>
					{#if editingSource.index >= 0 && hasSourceURLCredential(defaultCatalog?.sourceURLs?.[editingSource.index]) && !editingSource.clearToken}
						<button
							class="text-xs text-error hover:underline"
							onclick={() => {
								if (editingSource) {
									editingSource.clearToken = true;
									tokenExplicitlyCleared = true;
								}
							}}
						>
							Clear token
						</button>
					{/if}
				</div>
				{#if !editingSource.clearToken && editingSource.index >= 0 && hasSourceURLCredential(defaultCatalog?.sourceURLs?.[editingSource.index])}
					<p class="text-sm text-muted-content h-[39px]">
						{defaultCatalog?.sourceURLCredentials?.[
							defaultCatalog?.sourceURLs?.[editingSource.index]
						]}
					</p>
				{:else}
					<SensitiveInput
						name="catalog-source-token"
						placeholder={editingSource.clearToken
							? 'Enter a new value or leave empty to clear'
							: ''}
						bind:value={editingSource.token}
					/>
				{/if}
			</div>

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

			<div class="flex w-full justify-end gap-2">
				<button class="btn btn-secondary" disabled={saving} onclick={closeSourceDialog}
					>Cancel</button
				>
				<button
					class="btn btn-primary"
					disabled={saving}
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

							if (editingSource.clearToken && !editingSource.token) {
								sourceURLCredentials[newURL] = '';
							} else if (editingSource.token) {
								sourceURLCredentials[newURL] = editingSource.token;
							}

							if (Object.keys(sourceURLCredentials).length > 0) {
								updatingCatalog.sourceURLCredentials = sourceURLCredentials;
							}

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

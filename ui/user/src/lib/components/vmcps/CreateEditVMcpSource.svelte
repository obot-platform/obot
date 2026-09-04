<script lang="ts">
	import Select from '$lib/components/Select.svelte';
	import SensitiveInput from '$lib/components/SensitiveInput.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import { parseErrorContent } from '$lib/errors.js';
	import { AdminService } from '$lib/services';
	import type { GitCredential, VMcpRepository } from '$lib/services/admin/types';
	import { TriangleAlert, X } from '@lucide/svelte';

	type RepositoryCredentialType = 'none' | 'shared' | 'token';

	const repositoryCredentialOptions = [
		{ id: 'none', label: 'None' },
		{ id: 'shared', label: 'Choose existing' },
		{ id: 'token', label: 'Enter personal access token' }
	];

	interface Props {
		gitCredentials?: GitCredential[];
		vmcpRepositories?: VMcpRepository[];
		onSaved?: (repository: VMcpRepository) => void | Promise<void>;
	}

	let { gitCredentials = [], vmcpRepositories = [], onSaved }: Props = $props();

	let sourceDialog = $state<HTMLDialogElement | undefined>(undefined);
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
	let editingVMcpRepository = $derived(
		editingSource?.repositoryID
			? vmcpRepositories.find((repository) => repository.id === editingSource?.repositoryID)
			: undefined
	);
	let existingVMcpRepositoryToken = $derived(
		editingSource?.value.trim() === editingVMcpRepository?.repoURL
			? (editingVMcpRepository?.sourceURLCredentials?.[editingVMcpRepository.repoURL] ?? '')
			: ''
	);
	let existingSourceHasCredential = $derived(
		Boolean(
			editingSource &&
			editingSource.index >= 0 &&
			(hasVMcpRepositoryToken(editingVMcpRepository) ||
				Boolean(editingVMcpRepository?.gitCredentialID))
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
					(!hasVMcpRepositoryToken(editingVMcpRepository) ||
						editingSource.value.trim() !== editingVMcpRepository?.repoURL)))
		)
	);

	function sourceHost(value: string): string {
		try {
			return new URL(value.includes('://') ? value : `https://${value}`).host.toLowerCase();
		} catch {
			return '';
		}
	}

	function handleVMcpSourceURLInput() {
		if (!editingSource?.gitCredentialID) return;
		const selectedCredential = gitCredentials.find(
			(credential) => credential.id === editingSource?.gitCredentialID
		);
		const host = sourceHost(editingSource.value);
		if (selectedCredential && host && host !== selectedCredential.host.toLowerCase()) {
			editingSource.gitCredentialID = '';
		}
	}

	function hasVMcpRepositoryToken(repository: VMcpRepository | undefined): boolean {
		if (!repository) return false;
		const token = repository.sourceURLCredentials?.[repository.repoURL];
		return token !== undefined && token !== '';
	}

	function closeSourceDialog() {
		editingSource = undefined;
		sourceError = undefined;
		saving = false;
		sourceDialog?.close();
	}

	export function openAdd() {
		editingSource = {
			index: -1,
			value: '',
			name: '',
			ref: 'main',
			token: '',
			gitCredentialID: '',
			credentialType: 'none'
		};
		sourceError = undefined;
		sourceDialog?.showModal();
	}

	export function openEdit(repository: VMcpRepository) {
		editingSource = {
			index: vmcpRepositories.findIndex((candidate) => candidate.id === repository.id),
			value: repository.repoURL,
			name: repository.displayName,
			ref: repository.ref,
			token: '',
			gitCredentialID: repository.gitCredentialID ?? '',
			credentialType: repository.gitCredentialID
				? 'shared'
				: hasVMcpRepositoryToken(repository)
					? 'token'
					: 'none',
			repositoryID: repository.id
		};
		sourceError = undefined;
		sourceDialog?.showModal();
	}

	async function saveSource() {
		if (!editingSource) {
			return;
		}

		saving = true;
		sourceError = undefined;

		try {
			const repoURL = editingSource.value.trim();
			const token = editingSource.token.trim();
			const manifest: Parameters<typeof AdminService.createVMcpRepository>[0] = {
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
						hasVMcpRepositoryToken(editingVMcpRepository)))
			) {
				manifest.sourceURLCredentials = { [repoURL]: '' };
			}
			const response = editingSource.repositoryID
				? await AdminService.updateVMcpRepository(editingSource.repositoryID, manifest)
				: await AdminService.createVMcpRepository(manifest);
			await onSaved?.(response);
			closeSourceDialog();
		} catch (error) {
			sourceError = parseErrorContent(error).message;
		} finally {
			saving = false;
		}
	}
</script>

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
					<label for="vmcp-source-name" class="flex-1 text-sm font-light capitalize">Name </label>
					<input id="vmcp-source-name" bind:value={editingSource.name} class="text-input-filled" />
				</div>
				<div class="flex flex-col gap-1">
					<label for="vmcp-source-url" class="flex-1 text-sm font-light capitalize"
						>Source URL
					</label>
					<input
						id="vmcp-source-url"
						bind:value={editingSource.value}
						oninput={handleVMcpSourceURLInput}
						class="text-input-filled"
					/>
				</div>
				<div class="flex flex-col gap-1">
					<label for="vmcp-source-ref" class="flex-1 text-sm font-light capitalize"
						>Reference
					</label>
					<input id="vmcp-source-ref" bind:value={editingSource.ref} class="text-input-filled" />
					<span class="text-muted-content text-xs"
						>The branch, commit SHA, or tag to index and pull vMCPs from.</span
					>
				</div>
				<div class="flex flex-col gap-2">
					<div class="flex flex-col gap-1">
						<div class="flex items-center justify-between gap-4">
							<span id="vmcp-source-credential-label" class="flex-1 text-sm font-light capitalize">
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
							id="vmcp-source-credential-type"
							class="bg-base-200"
							options={repositoryCredentialOptions}
							selected={editingSource.credentialType}
							ariaLabelledby="vmcp-source-credential-label"
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
									if (hasVMcpRepositoryToken(editingVMcpRepository)) {
										editingSource.clearToken = true;
									}
								}
							}}
						/>
					</div>
					{#if editingSource.credentialType === 'shared'}
						<div class="flex flex-col gap-1">
							<Select
								id="vmcp-source-git-credential"
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
							<label for="vmcp-source-token" class="sr-only">Personal Access Token</label>
							{#if credentialLocked && existingVMcpRepositoryToken}
								<input
									id="vmcp-source-token"
									type="text"
									readonly
									aria-readonly="true"
									data-1p-ignore
									value={existingVMcpRepositoryToken}
									class="text-sm text-muted-content w-full border-none bg-transparent p-0 outline-none focus:ring-0 min-h-10"
								/>
							{:else}
								<SensitiveInput
									name="vmcp-source-token"
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
					onclick={saveSource}
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

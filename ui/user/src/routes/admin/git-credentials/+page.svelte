<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import SensitiveInput from '$lib/components/SensitiveInput.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { HttpError } from '$lib/errors';
	import { AdminService, type GitCredential, type GitCredentialManifest } from '$lib/services';
	import { errors, profile } from '$lib/stores';
	import { KeyRound, Pencil, Plus, Trash2, TriangleAlert, X } from '@lucide/svelte';
	import { untrack } from 'svelte';

	const { data }: { data: { gitCredentials: GitCredential[] } } = $props();

	let gitCredentials = $state<GitCredential[]>(untrack(() => data.gitCredentials));
	let editingCredential = $state<GitCredential>();
	let deletingCredential = $state<GitCredential>();
	let viewingCredential = $state<GitCredential>();
	let displayName = $state('');
	let host = $state('');
	let token = $state('');
	let clearToken = $state(false);
	let formError = $state('');
	let saving = $state(false);
	let dialog = $state<HTMLDialogElement>();
	let isReadonly = $derived(profile.current.isAdminReadonly?.());
	let showExistingToken = $derived(Boolean(editingCredential?.tokenConfigured) && !clearToken);
	let tokenRequired = $derived(!editingCredential?.tokenConfigured || clearToken);
	let tableData = $derived(
		gitCredentials.map((credential) => ({
			...credential,
			usedBy: [
				...(credential.uses.skillRepositories ?? []),
				...(credential.uses.mcpCatalogs ?? []),
				...(credential.uses.systemMcpCatalogs ?? [])
			].length
		}))
	);

	function useGroups(credential?: GitCredential) {
		return [
			{ label: 'Skill Repositories', uses: credential?.uses.skillRepositories ?? [] },
			{ label: 'MCP Catalogs', uses: credential?.uses.mcpCatalogs ?? [] },
			{ label: 'System MCP Catalogs', uses: credential?.uses.systemMcpCatalogs ?? [] }
		].filter((group) => group.uses.length > 0);
	}

	function hasUses(credential?: GitCredential) {
		return useGroups(credential).length > 0;
	}

	function openCreate() {
		editingCredential = undefined;
		displayName = '';
		host = '';
		token = '';
		clearToken = false;
		formError = '';
		dialog?.showModal();
	}

	function openEdit(credential: GitCredential) {
		editingCredential = credential;
		displayName = credential.displayName;
		host = credential.host;
		token = '';
		clearToken = false;
		formError = '';
		dialog?.showModal();
	}

	function openDelete(credential: GitCredential) {
		deletingCredential = credential;
	}

	function openUses(credential: GitCredential) {
		viewingCredential = credential;
	}

	function closeDialog() {
		dialog?.close();
	}

	function handleDialogClose() {
		token = '';
		clearToken = false;
		formError = '';
	}

	async function saveCredential() {
		const normalizedToken = token.trim();
		if (isReadonly || !displayName.trim() || !host.trim() || (tokenRequired && !normalizedToken)) {
			return;
		}

		saving = true;
		formError = '';
		try {
			const input: GitCredentialManifest = {
				displayName: displayName.trim(),
				host: host.trim()
			};
			if (normalizedToken) input.token = normalizedToken;

			const saved = editingCredential
				? await AdminService.updateGitCredential(editingCredential.id, input, {
						dontLogErrors: true
					})
				: await AdminService.createGitCredential(input, { dontLogErrors: true });

			const index = gitCredentials.findIndex((credential) => credential.id === saved.id);
			gitCredentials =
				index === -1
					? [saved, ...gitCredentials]
					: gitCredentials.map((credential) => (credential.id === saved.id ? saved : credential));
			closeDialog();
		} catch (error) {
			formError = error instanceof Error ? error.message : 'Unable to save Git credential';
		} finally {
			saving = false;
		}
	}
</script>

<Layout title="Git Credentials" showBackButton>
	{#if gitCredentials.length === 0}
		<div class="mt-12 flex w-md max-w-full flex-col items-center gap-4 self-center text-center">
			<KeyRound class="text-muted-content size-24 opacity-25" />
			<h4 class="text-muted-content text-lg font-semibold">No Git credentials</h4>
			<p class="text-muted-content text-sm font-light">
				Create a host-bound credential to reuse a personal access token across Git repositories.
			</p>
			{#if !isReadonly}
				<button class="btn btn-primary flex items-center gap-1 text-sm" onclick={openCreate}>
					<Plus class="size-4" />
					Create Git Credential
				</button>
			{/if}
		</div>
	{:else}
		<Table
			data={tableData}
			fields={['displayName', 'host', 'usedBy']}
			headers={[
				{ title: 'Name', property: 'displayName' },
				{ title: 'Host', property: 'host' },
				{ title: 'Used By', property: 'usedBy' }
			]}
			sortable={['displayName', 'host']}
			filterable={['displayName', 'host']}
			onClickRow={(row) => openEdit(row)}
		>
			{#snippet onRenderColumn(field, credential)}
				{#if field === 'displayName'}
					<span class="flex items-center gap-2">
						{credential.displayName}
						{#if hasUses(credential)}
							<button
								type="button"
								class="pill-warning border-warning/30 hover:border-warning/60 hover:bg-warning/20 focus-visible:ring-warning/40 cursor-pointer border transition-colors focus-visible:ring-2 focus-visible:outline-none"
								onclick={(event) => {
									event.stopPropagation();
									openUses(credential);
								}}
							>
								In Use
							</button>
						{/if}
					</span>
				{:else if field === 'host'}
					{credential.host}
				{:else if field === 'usedBy'}
					{#if credential.usedBy}
						<button
							type="button"
							class="text-left hover:underline"
							onclick={(event) => {
								event.stopPropagation();
								openUses(credential);
							}}
						>
							{credential.usedBy}
							{credential.usedBy > 1 ? 'Sources' : 'Source'}
						</button>
					{:else}
						<span class="text-muted-content">—</span>
					{/if}
				{/if}
			{/snippet}
			{#snippet actions(credential)}
				{#if !isReadonly}
					<IconButton
						onclick={(event) => {
							event.stopPropagation();
							openEdit(credential);
						}}
					>
						<Pencil class="size-4" />
					</IconButton>
					<div
						class="shrink-0"
						use:tooltip={hasUses(credential)
							? {
									text: 'This credential is currently in use and cannot be deleted.',
									placement: 'left'
								}
							: undefined}
					>
						<IconButton
							variant="danger"
							disabled={hasUses(credential)}
							onclick={(event) => {
								event.stopPropagation();
								openDelete(credential);
							}}
						>
							<Trash2 class="size-4" />
						</IconButton>
					</div>
				{/if}
			{/snippet}
		</Table>
	{/if}

	{#snippet rightNavActions()}
		{#if !isReadonly && gitCredentials.length > 0}
			<button class="btn btn-primary flex items-center gap-1 text-sm" onclick={openCreate}>
				<Plus class="size-4" />
				Create Git Credential
			</button>
		{/if}
	{/snippet}
</Layout>

<dialog bind:this={dialog} class="dialog" onclose={handleDialogClose}>
	<div class="dialog-container w-full max-w-md p-4">
		<h3 class="dialog-title">
			{editingCredential ? 'Edit Git Credential' : 'Create Git Credential'}
			<IconButton onclick={closeDialog} class="btn-sm dialog-close-btn">
				<X class="size-5" />
			</IconButton>
		</h3>

		<div class="my-4 flex flex-col gap-4">
			<div class="flex flex-col gap-1">
				<label for="git-credential-name" class="text-sm font-light">Name</label>
				<input
					id="git-credential-name"
					bind:value={displayName}
					disabled={isReadonly}
					class="text-input-filled"
				/>
			</div>
			<div class="flex flex-col gap-1">
				<label for="git-credential-host" class="text-sm font-light">Git host</label>
				<input
					id="git-credential-host"
					bind:value={host}
					disabled={isReadonly || Boolean(editingCredential)}
					placeholder="github.com"
					class="text-input-filled"
				/>
				<span class="text-muted-content text-xs">Enter a hostname without a scheme or path.</span>
			</div>
			<div class="flex flex-col gap-1">
				<div class="flex items-center justify-between gap-4">
					<label for="git-credential-token" class="text-sm font-light">
						Personal access token
					</label>
					{#if showExistingToken && !isReadonly}
						<button
							type="button"
							class="text-xs text-error hover:underline"
							onclick={() => {
								clearToken = true;
								token = '';
							}}
						>
							Clear token
						</button>
					{/if}
				</div>
				{#if showExistingToken}
					<input
						id="git-credential-token"
						type="text"
						readonly
						aria-readonly="true"
						data-1p-ignore
						value="****"
						class="text-sm text-muted-content w-full border-none bg-transparent p-0 outline-none focus:ring-0 min-h-10"
					/>
				{:else}
					<SensitiveInput name="git-credential-token" bind:value={token} disabled={isReadonly} />
				{/if}
			</div>
		</div>

		{#if formError}
			<div class="mb-4 flex items-start gap-2 text-error">
				<TriangleAlert class="size-5 shrink-0" />
				<span class="text-sm break-all">{formError}</span>
			</div>
		{/if}

		<div class="flex justify-end gap-2">
			<button class="btn btn-secondary" disabled={saving} onclick={closeDialog}>Cancel</button>
			<button
				class="btn btn-primary"
				disabled={isReadonly ||
					saving ||
					!displayName.trim() ||
					!host.trim() ||
					(tokenRequired && !token.trim())}
				onclick={saveCredential}
			>
				{saving ? 'Saving...' : editingCredential ? 'Save' : 'Create'}
			</button>
		</div>
	</div>
	<form class="dialog-backdrop">
		<button type="button" onclick={closeDialog}>close</button>
	</form>
</dialog>

{#snippet useSections(credential: GitCredential)}
	{#each useGroups(credential) as group (group.label)}
		<section class="flex flex-col gap-1">
			<h4 class="text-xs font-semibold">{group.label}</h4>
			<ul class="bg-surface-1 divide-border divide-y rounded-md border">
				{#each group.uses as use (`${use.id}:${use.displayName ?? ''}`)}
					<li class="px-3 py-2 text-sm break-all">{use.displayName || use.id}</li>
				{/each}
			</ul>
		</section>
	{/each}
{/snippet}

{#snippet deleteNote()}
	{#if hasUses(deletingCredential)}
		<div class="flex w-full flex-col gap-2 text-left">
			<p>This credential cannot be deleted because it is used by:</p>
			{@render useSections(deletingCredential!)}
		</div>
	{:else}
		<p>This action is permanent and cannot be undone.</p>
	{/if}
{/snippet}

{#snippet usesNote()}
	{#if viewingCredential}
		<div class="flex w-full flex-col gap-2 text-left">
			{@render useSections(viewingCredential)}
		</div>
	{/if}
{/snippet}

<Confirm
	title="Credential Uses"
	msg={`${viewingCredential?.displayName ?? 'This credential'} is used by:`}
	note={usesNote}
	type="info"
	show={Boolean(viewingCredential)}
	cancelText="Close"
	oncancel={() => (viewingCredential = undefined)}
	classes={{ note: 'w-full' }}
/>

<Confirm
	msg={`Delete ${deletingCredential?.displayName ?? 'this Git credential'}?`}
	note={deleteNote}
	show={Boolean(deletingCredential)}
	loading={saving}
	disabled={hasUses(deletingCredential)}
	onsuccess={async () => {
		if (!deletingCredential) return;
		saving = true;
		try {
			await AdminService.deleteGitCredential(deletingCredential.id, { dontLogErrors: true });
			gitCredentials = gitCredentials.filter(
				(credential) => credential.id !== deletingCredential?.id
			);
			deletingCredential = undefined;
		} catch (error) {
			errors.append(`Failed to delete Git credential: ${error}`);
			if (error instanceof HttpError && error.statusCode === 409) {
				gitCredentials = gitCredentials.map((credential) =>
					credential.id === deletingCredential?.id
						? {
								...credential,
								uses: {
									skillRepositories: [{ id: 'resource', displayName: 'Unknown resource' }],
									mcpCatalogs: [],
									systemMcpCatalogs: []
								}
							}
						: credential
				);
				deletingCredential = undefined;
			}
		} finally {
			saving = false;
		}
	}}
	oncancel={() => (deletingCredential = undefined)}
/>

<svelte:head>
	<title>Obot | Git Credentials</title>
</svelte:head>
